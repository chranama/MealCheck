# Mac supervision and scoped outage evidence

Recorded October 7, 2026. Commands below describe historical verification; see [current operations](../operations/macos.md) for instructions.

## Current-session supervision evidence (2026-10-07)

A real LaunchAgent was bootstrapped in `gui/501` using
`MEALCHECK_LAUNCHAGENT_PATH=/tmp/controller-lab/dev.mealcheck.controller.lab.plist`.
The process ran natively with the Docker Desktop Unix endpoint and private
controller-state directory. This temporary plist was deliberately not installed
under `~/Library/LaunchAgents`; it proves current-session bootstrap only and
will not be automatically discovered at the next login. Persistent installation
requires the default account LaunchAgents path and durable paths described above.

A SIGKILL of the managed controller changed PID101 to PID787. The socket served
persisted `Ready` status after 0.138 seconds; this measures supervisor/process
availability, not fresh workload observation. A later status check confirmed
fresh observation at `2026-10-07T15:37:18.794362Z`, generation1, Ready. All six
managed resource IDs (three containers, two volumes, one network) stayed identical.
The synthetic completed run `run_552b21f7b10c5eaf60b9ae80` remained completed.
The daemon was left running for subsequent failure scenarios. No Docker Desktop
shutdown, login/logout, reboot, native production service change, or shared ingress
change was performed.

## Scoped engine-connectivity outage

`python3 deploy/controller/verification/test-engine-outage.py` runs a reproducible lab-only
outage without stopping Docker Desktop or other workloads. It temporarily routes
the dedicated LaunchAgent through `engine-outage.py`, closes its active forwarding
streams with SIGUSR1, and restores forwarding with SIGUSR2. The `finally` cleanup
restores the original direct socket endpoint. Run only with the lab stack Ready.

Measured 2026-10-07: loss detected in 5.033s, phase `Degraded`, resource/dependency/API
conditions `Unknown` with reason `EngineObservationUnavailable`. Direct Docker Engine
29.8.2 remained healthy. After restoring forwarding, fresh Ready observation took
4.825s; generation 3 and all managed resource IDs stayed unchanged. The original
LaunchAgent endpoint was restored and fresh Ready was verified afterward. This
proves controller handling of engine connectivity loss; it does not prove actual
Docker Desktop shutdown/restart or VM reboot behavior.

## Final teardown evidence

Deletion converged to Deleted, tombstone true, generation4. No managed containers
or network remained; only the original database/artifact volumes were retained
with matching incarnations. Repeated delete retained generation4. The temporary
LaunchAgent was then uninstalled; the controller state and workload data remain.
The local build registry remains available, outside the controller lifecycle.
