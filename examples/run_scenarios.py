#!/usr/bin/env python3

import json
import os
import sys
import urllib.error
import urllib.request


BASE_URL = os.environ.get("BASE_URL", "http://localhost:8080").rstrip("/")
SCENARIO_PATH = os.path.join(os.path.dirname(__file__), "scenarios.json")


def request_json(method, path, expected_status, payload=None):
    data = None if payload is None else json.dumps(payload).encode("utf-8")
    headers = {"Content-Type": "application/json"} if data is not None else {}
    request = urllib.request.Request(
        BASE_URL + path,
        data=data,
        headers=headers,
        method=method,
    )

    try:
        with urllib.request.urlopen(request, timeout=5) as response:
            body = json.load(response)
            status = response.status
    except urllib.error.HTTPError as error:
        body = error.read().decode("utf-8")
        raise RuntimeError(
            f"{method} {path} returned {error.code}, expected {expected_status}: {body}"
        ) from error
    except urllib.error.URLError as error:
        raise RuntimeError(
            f"cannot reach {BASE_URL}; start the API with docker compose up --build: {error.reason}"
        ) from error

    if status != expected_status:
        raise RuntimeError(
            f"{method} {path} returned {status}, expected {expected_status}: {body}"
        )
    return body


def sessions_by_id(state):
    return {session["id"]: session for session in state["sessions"]}


def chargers_by_id(state):
    return {charger["id"]: charger for charger in state["chargers"]}


def connectors_by_id(charger):
    return {connector["id"]: connector for connector in charger["connectors"]}


def require_power(session, expected_power_kw):
    actual_power_kw = session["assignedPowerKw"]
    if abs(actual_power_kw - expected_power_kw) > 1e-6:
        raise RuntimeError(
            f"session {session['id']} received {actual_power_kw} kW, expected {expected_power_kw} kW"
        )


def require_bess(state, expected_power_kw, expected_soc_percent, expected_mode):
    bess = state.get("bess")
    if bess is None:
        raise RuntimeError("OPS state does not include the configured BESS")
    if abs(bess["currentPowerKw"] - expected_power_kw) > 1e-6:
        raise RuntimeError(
            f"BESS power is {bess['currentPowerKw']} kW, expected {expected_power_kw} kW"
        )
    if abs(bess["socPercent"] - expected_soc_percent) > 1e-6:
        raise RuntimeError(
            f"BESS SoC is {bess['socPercent']}%, expected {expected_soc_percent}%"
        )
    if bess["mode"] != expected_mode:
        raise RuntimeError(
            f"BESS mode is {bess['mode']}, expected {expected_mode}"
        )


def main():
    with open(SCENARIO_PATH, encoding="utf-8") as scenario_file:
        scenario = json.load(scenario_file)

    station = scenario["station"]
    first = scenario["sessions"]["first"]
    second = scenario["sessions"]["second"]
    after_connector_restore = scenario["sessions"]["afterConnectorRestore"]
    before_charger_outage = scenario["sessions"]["beforeChargerOutage"]
    expected = scenario["expectedPowerKw"]
    bess = scenario["bess"]
    bess_expected = scenario["bessExpected"]

    health = request_json("GET", "/health", 200)
    if health.get("status") != "ok":
        raise RuntimeError(f"unexpected health response: {health}")
    print("PASS health")

    state = request_json("PUT", "/api/v1/station/config", 200, station)
    if state["stationId"] != station["id"]:
        raise RuntimeError(f"configured station ID is incorrect: {state['stationId']}")
    print("PASS configure representative station")

    started = request_json("POST", "/api/v1/sessions", 201, first)
    require_power(started, expected["firstAlone"])
    print("PASS first session receives full demand")

    started = request_json("POST", "/api/v1/sessions", 201, second)
    require_power(started, expected["shared"])
    print("PASS second session triggers constrained sharing")

    state = request_json("GET", "/api/v1/station", 200)
    sessions = sessions_by_id(state)
    require_power(sessions[first["id"]], expected["shared"])
    require_power(sessions[second["id"]], expected["shared"])
    if state["gridImportKw"] > station["gridCapacityKw"]:
        raise RuntimeError("grid import exceeds configured capacity")
    print("PASS OPS state shows fair 200/200 kW sharing")

    updated = request_json(
        "PATCH",
        f"/api/v1/sessions/{first['id']}",
        200,
        scenario["updates"]["limitFirst"],
    )
    require_power(updated, expected["limitedFirst"])
    state = request_json("GET", "/api/v1/station", 200)
    sessions = sessions_by_id(state)
    require_power(sessions[second["id"]], expected["afterUpdateSecond"])
    print("PASS demand update redistributes power to 100/300 kW")

    request_json(
        "PATCH",
        f"/api/v1/sessions/{first['id']}",
        200,
        scenario["updates"]["restoreFirst"],
    )
    state = request_json("GET", "/api/v1/station", 200)
    sessions = sessions_by_id(state)
    require_power(sessions[first["id"]], expected["shared"])
    require_power(sessions[second["id"]], expected["shared"])
    print("PASS restored demand returns to fair sharing")

    state = request_json(
        "PATCH",
        f"/api/v1/connectors/{first['connectorId']}",
        200,
        {"status": "unavailable"},
    )
    sessions = sessions_by_id(state)
    if set(sessions) != {second["id"]}:
        raise RuntimeError(f"unexpected sessions after connector outage: {sorted(sessions)}")
    require_power(sessions[second["id"]], expected["afterOutageSecond"])
    connector = connectors_by_id(chargers_by_id(state)["charger-1"])[
        first["connectorId"]
    ]
    if connector["status"] != "unavailable" or connector["occupied"]:
        raise RuntimeError(f"unexpected unavailable connector state: {connector}")
    print("PASS connector outage ends its session and redistributes power")

    state = request_json(
        "PATCH",
        f"/api/v1/connectors/{first['connectorId']}",
        200,
        {"status": "available"},
    )
    connector = connectors_by_id(chargers_by_id(state)["charger-1"])[
        first["connectorId"]
    ]
    if connector["status"] != "available":
        raise RuntimeError(f"connector was not restored: {connector}")

    started = request_json("POST", "/api/v1/sessions", 201, after_connector_restore)
    require_power(started, expected["shared"])
    state = request_json("GET", "/api/v1/station", 200)
    sessions = sessions_by_id(state)
    require_power(sessions[after_connector_restore["id"]], expected["shared"])
    require_power(sessions[second["id"]], expected["shared"])
    print("PASS restored connector accepts a new session")

    state = request_json(
        "DELETE", f"/api/v1/sessions/{after_connector_restore['id']}", 200
    )
    sessions = sessions_by_id(state)
    if set(sessions) != {second["id"]}:
        raise RuntimeError(f"unexpected sessions after stop: {sorted(sessions)}")
    require_power(sessions[second["id"]], expected["afterStopSecond"])
    print("PASS session stop redistributes power immediately")

    started = request_json("POST", "/api/v1/sessions", 201, before_charger_outage)
    require_power(started, expected["shared"])

    state = request_json(
        "PATCH",
        "/api/v1/chargers/charger-1",
        200,
        {"status": "unavailable"},
    )
    sessions = sessions_by_id(state)
    if set(sessions) != {second["id"]}:
        raise RuntimeError(f"unexpected sessions after charger outage: {sorted(sessions)}")
    require_power(sessions[second["id"]], expected["afterOutageSecond"])
    charger = chargers_by_id(state)["charger-1"]
    if charger["status"] != "unavailable" or charger["currentPowerKw"] != 0:
        raise RuntimeError(f"unexpected unavailable charger state: {charger}")
    print("PASS charger outage ends attached sessions and redistributes power")

    state = request_json(
        "PATCH",
        "/api/v1/chargers/charger-1",
        200,
        {"status": "available"},
    )
    if chargers_by_id(state)["charger-1"]["status"] != "available":
        raise RuntimeError("charger was not restored")
    print("PASS charger restored")

    state = request_json("GET", "/api/v1/station", 200)
    sessions = sessions_by_id(state)
    if set(sessions) != {second["id"]}:
        raise RuntimeError(f"unexpected final sessions: {sorted(sessions)}")
    require_power(sessions[second["id"]], expected["afterOutageSecond"])
    print("PASS final OPS state")

    bess_station = {**station, "bess": bess}
    state = request_json("PUT", "/api/v1/station/config", 200, bess_station)
    require_bess(
        state,
        bess_expected["chargingWithoutSessionsKw"],
        bess["socPercent"],
        "charging",
    )
    if state["gridImportKw"] != 200:
        raise RuntimeError(
            f"grid import while charging BESS is {state['gridImportKw']} kW, expected 200 kW"
        )
    print("PASS spare grid capacity charges the BESS")

    started = request_json("POST", "/api/v1/sessions", 201, first)
    require_power(started, expected["firstAlone"])
    state = request_json("GET", "/api/v1/station", 200)
    require_bess(
        state,
        bess_expected["chargingAfterFirstSessionKw"],
        bess["socPercent"],
        "charging",
    )
    if state["gridImportKw"] != station["gridCapacityKw"]:
        raise RuntimeError("BESS charging did not yield spare grid power to EV demand")
    print("PASS EV demand takes priority over BESS charging")

    started = request_json("POST", "/api/v1/sessions", 201, second)
    require_power(started, bess_expected["boostedSessionKw"])
    state = request_json("GET", "/api/v1/station", 200)
    sessions = sessions_by_id(state)
    require_power(sessions[first["id"]], bess_expected["boostedSessionKw"])
    require_power(sessions[second["id"]], bess_expected["boostedSessionKw"])
    require_bess(
        state,
        bess_expected["dischargingKw"],
        bess["socPercent"],
        "discharging",
    )
    if state["gridImportKw"] > station["gridCapacityKw"]:
        raise RuntimeError("BESS boost caused grid import to exceed capacity")
    print("PASS BESS boost supplies 200 kW above grid capacity")

    state = request_json(
        "POST", "/api/v1/simulation/tick", 200, {"elapsedSeconds": 15 * 60}
    )
    require_bess(
        state,
        bess_expected["dischargingKw"],
        bess_expected["socAfterFirstTickPercent"],
        "discharging",
    )
    print("PASS simulation tick updates BESS SoC deterministically")

    state = request_json(
        "POST", "/api/v1/simulation/tick", 200, {"elapsedSeconds": 15 * 60}
    )
    sessions = sessions_by_id(state)
    require_bess(
        state,
        0,
        bess_expected["minimumSocPercent"],
        "idle",
    )
    require_power(sessions[first["id"]], bess_expected["sessionAtMinimumSocKw"])
    require_power(sessions[second["id"]], bess_expected["sessionAtMinimumSocKw"])
    print("PASS minimum SoC stops discharge and recomputes EV allocations")


if __name__ == "__main__":
    try:
        main()
    except (KeyError, OSError, ValueError, RuntimeError) as error:
        print(f"FAIL {error}", file=sys.stderr)
        sys.exit(1)
