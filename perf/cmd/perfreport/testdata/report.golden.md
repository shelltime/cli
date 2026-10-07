<!-- shelltime-track-perf -->
## ⏱️ `shelltime track` performance

⚠️ **1 scenario got slower**: more than 10% in wall time · ❌ 1 scenario produced no result on head

Comparing `main` @ abc1234 → #42 @ def5678. Each scenario execs the binary the way the shell hooks do; values are the median of 10 interleaved rounds. Lower is better.

| Scenario | base | head | Δ | p | |
|---|--:|--:|--:|--:|:-:|
| `Startup/floor` | 1.20 ms ±2% | 1.20 ms ±2% | ~ | 1.000 | A/A |
| `Startup/version` | 5.39 ms ±2% | 5.44 ms ±2% | ~ | 0.123 | ✅ |
| `Track/daemon/pre` | 5.99 ms ±2% | 7.19 ms ±2% | +20.00% | 0.000 | ⚠️ |
| `Track/daemon/post` | 6.19 ms ±2% | 6.56 ms ±2% | +6.00% | 0.000 | ✅ |
| `Track/direct/pre` | 5.59 ms ±2% | 4.19 ms ±2% | -25.00% | 0.000 | 🚀 |
| `Track/direct/post-sync` | 200 µs ±2% | 299 µs ±2% | +50.00% | 0.000 | ✅ |
| `Track/direct/new` | — | 2.99 ms ±2% | new | | 🆕 |
| `Track/direct/post` | 9.38 ms ±2% | — | | | ❌ |

<details><summary>CPU, tail latency and memory</summary>

Per exec, base → head (Δ when significant).

| Scenario | p95 wall | user CPU | sys CPU | peak RSS |
|---|--:|--:|--:|--:|
| `Startup/floor` | 1.38 ms → 1.38 ms | 659 µs → 659 µs | 539 µs → 539 µs | 16.5 MiB → 16.5 MiB |
| `Startup/version` | 6.20 ms → 6.26 ms | 2.96 ms → 2.99 ms | 2.43 ms → 2.45 ms | 16.5 MiB → 16.7 MiB (+1.00%) |
| `Track/daemon/pre` | 6.89 ms → 8.26 ms (+20.00%) | 3.29 ms → 3.95 ms (+20.00%) | 2.69 ms → 3.23 ms (+20.00%) | 16.5 MiB → 19.8 MiB (+20.00%) |
| `Track/daemon/post` | 7.12 ms → 7.54 ms (+6.00%) | 3.40 ms → 3.61 ms (+6.00%) | 2.78 ms → 2.95 ms (+6.00%) | 16.5 MiB → 17.5 MiB (+6.00%) |
| `Track/direct/pre` | 6.43 ms → 4.82 ms (-25.00%) | 3.07 ms → 2.31 ms (-25.00%) | 2.51 ms → 1.89 ms (-25.00%) | 16.5 MiB → 12.4 MiB (-25.00%) |
| `Track/direct/post-sync` | 230 µs → 344 µs (+50.00%) | 110 µs → 165 µs (+50.00%) | 90 µs → 135 µs (+50.00%) | 16.5 MiB → 24.8 MiB (+50.00%) |
| `Track/direct/new` | — | — | — | — |
| `Track/direct/post` | — | — | — | — |

</details>

<sub>Δ compares medians; ~ means no significant difference (Mann-Whitney U, p ≥ 0.05). A scenario is flagged when it is more than 10% and more than 250µs slower with p < 0.05. `Startup/floor` runs `true`, not shelltime: it is an A/A check of runner noise. Runner: linux/amd64, Test CPU @ 2.10GHz. Go go1.27.1.</sub>
