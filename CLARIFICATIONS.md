# How should constrained power be shared between sessions?

Same guidance as before: fair/equal sharing across active sessions by default, each capped by its own vehicle/connector limits, and adjusted so no session drops below its useful minimum (per the admission/minimum policy you decide on). Document it as your assumption; that's the simplest defensible policy and matches the brief's own example of sessions splitting power when a second vehicle arrives.

# Should unused capacity be redistributed?

Yes. If one session isn't using its full allocated share (e.g. its vehicle caps out below what it was offered, or it finishes/leaves), that freed-up power should be redistributed to the other active sessions rather than sitting idle. This ties back to the brief's example: when the first vehicle leaves, the second should be free to take all the power it wants, that's the same principle applied continuously, not just on disconnect.

# Is charger power shared across its connectors?

Yes, that's an important constraint from the brief's example: a charger has its own max power rating (e.g. 300kW), and if it has multiple connectors, sessions on those connectors share that charger-level limit, in addition to the overall station grid limit. So you've actually got two layers of constraint to respect: the station's total grid capacity, and each charger's own capacity across its connectors. Worth modeling explicitly since it's called out in the sample station setup.

# What limits define a session’s effective maximum power?

A session's effective max power is the minimum of several constraints stacked together:

The vehicle's own requested/limit power (its charging curve, which changes over time, e.g. as SoC rises)
The connector's max power (e.g. a given CCS connector's rating)
The charger's max power, shared across its connectors if multiple are active
The station's overall grid capacity, shared across all chargers
Whatever share the fair-allocation policy assigns it given other concurrent sessions
So it's the tightest of all these at any moment, and it can shift in real time as any one of them changes (vehicle curve, other sessions starting/stopping, BESS kicking in). That's the core computation your system needs to get right.

# What happens when there is not enough power to meet a session’s minimum useful threshold?

We covered this earlier: it's a policy decision you need to make and document. Reasonable options: don't admit/start the session until enough power frees up, pause an already-running session that's been squeezed below its minimum, or deprioritize it in favor of sessions that can be served properly. Whichever you pick, state the trade-off clearly, that's more important than which option you choose.

# What should happen to sessions when a charger or connector becomes unavailable?

A few things need to happen:

Any session on that connector/charger should be ended (or flagged as failed) since it can no longer deliver power.
The power it was using should be freed up and redistributed to the remaining active sessions.
OPS needs visibility into the fact that connector/charger is now unavailable (that's part of the live-state requirement), since they may need to dispatch a "flying doctor" or flag it for maintenance.
It's a good edge case to explicitly handle: unavailability isn't just "no session," it should actively remove that capacity from your allocation pool and surface itself in the state you expose to OPS.

# What exactly should OPS see in the station-state response?

At a station level, OPS needs to see roughly:

Overall station status: total grid capacity, how much is currently allocated/in use, how much headroom is left
Per-charger status: availability (available, occupied, faulted/unavailable), current power draw
Per-connector status: same, plus which one is active, its current allocated power
Per-session info where relevant: what's being delivered vs requested, so OPS can spot a vehicle being throttled
BESS status if present: current SoC, whether it's charging/discharging and how much
The point is OPS should be able to tell at a glance "is this station healthy, is anything faulted, is anyone being underserved, is capacity being used well" without digging into logs. Exact structure/format of how you expose that is your call, that's the interface design piece I'll leave to you.

# For BESS, should EV charging always take priority, and how should SoC be updated?

Priority: yes, EV charging should take priority. The BESS charges opportunistically from genuinely spare grid capacity (never at the expense of an EV session), and discharges to help EVs when grid capacity is insufficient. EVs are the core service; the battery is a supporting tool.

SoC updates: keep it simple, track SoC as a function of power flow over time (energy in when charging, energy out when discharging, converted against its kWh capacity), just enough to know its current state and enforce the >10% floor. No need to model efficiency losses or anything more sophisticated than a basic running energy balance.

# Should older sessions or sessions with lower State of Charge receive priority, or should all sessions be treated equally?

Treat all sessions equally by default, that's consistent with the fair-sharing example in the brief and the simplest reasonable baseline. SoC-based or age-based prioritization is a legitimate advanced enhancement you could mention as a future improvement (e.g. "vehicles closer to empty could get priority to reduce anxiety/downtime"), but it's not required, and I wouldn't spend core time on it. If you want to explore it, treat it as a stretch goal after the basic equal-share logic is solid and documented.

# If fair-share allocation would push a session below its minimum, you need to decide how to handle it.

Right, that's a decision point I'd want you to make and document rather than something I'll dictate. A few reasonable directions, any of which would be defensible:

Cut off the lowest-priority/newest session so remaining sessions stay above their minimum, rather than spreading power so thin everyone's below useful threshold.
Queue it: let it wait for power to free up rather than start a session that can't get minimum viable power.
Give it the minimum and shrink others further if there's room, accepting slight unfairness.
Pick one, state the trade-off (fairness vs usability vs simplicity), and move on. It's a good edge case to call out explicitly in your write-up even if your implementation handles it simply.
