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


def require_power(session, expected_power_kw):
    actual_power_kw = session["assignedPowerKw"]
    if abs(actual_power_kw - expected_power_kw) > 1e-6:
        raise RuntimeError(
            f"session {session['id']} received {actual_power_kw} kW, expected {expected_power_kw} kW"
        )


def main():
    with open(SCENARIO_PATH, encoding="utf-8") as scenario_file:
        scenario = json.load(scenario_file)

    station = scenario["station"]
    first = scenario["sessions"]["first"]
    second = scenario["sessions"]["second"]
    expected = scenario["expectedPowerKw"]

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

    state = request_json("DELETE", f"/api/v1/sessions/{first['id']}", 200)
    sessions = sessions_by_id(state)
    if set(sessions) != {second["id"]}:
        raise RuntimeError(f"unexpected sessions after stop: {sorted(sessions)}")
    require_power(sessions[second["id"]], expected["afterStopSecond"])
    print("PASS session stop redistributes power immediately")

    state = request_json("GET", "/api/v1/station", 200)
    sessions = sessions_by_id(state)
    require_power(sessions[second["id"]], expected["afterStopSecond"])
    print("PASS final OPS state")


if __name__ == "__main__":
    try:
        main()
    except (KeyError, OSError, ValueError, RuntimeError) as error:
        print(f"FAIL {error}", file=sys.stderr)
        sys.exit(1)
