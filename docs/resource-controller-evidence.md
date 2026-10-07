# Resource Controller Evidence

Implementation scope: M0–M6. M7 native Linux host operation is deferred.

## M0 — Environment and workload contract

Date: 2026-10-07. Status: verified and independently reviewed.

Contract commit: `f591ac4`. PR: https://github.com/chranama/MealCheck/pull/2.

Observed macOS ARM64 with Go 1.26.4, 128 GiB RAM, 1.6 TiB free disk; Docker
Desktop 4.94.0 / Engine 29.8.2 linux/arm64 at the desktop Unix socket. Verified
local Qwen3-0.6B-Q4_K_M model availability and no listener on port 18080.
Commands: `uname -m`, `go version`, `sysctl -n hw.memsize`, `df -h`, Docker
context/info, model inventory, and workload configuration source inspection.

The independent review requested explicit M7 deferral and milestone evidence;
these corrections are included. Runtime/model image digest proof belongs to M1.
No production services or public ingress were changed.
