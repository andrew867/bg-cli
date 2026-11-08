# Egg probability models

All models are **Hypothetical** unless labeled otherwise. They are public-config arithmetic or fitted illustrations, not verified live player odds.

## Secret Room Egg raw pets table (fixture)

| Pet | Raw weight | Label |
| --- | --- | --- |
| Phantom Tiger | 90 | |
| Thorn Dragon | 10 | |
| Masquerade Agony | 0.0034 | Great |
| Huge Bat Hoodie Panda | 0.000015 | Insane |

Great/Insane are raw labels. Tiny decimals are raw fields — not proven percents.

## Models

1. **direct-percent** — treat weight as percent points (90 → 0.90). Plausible for 90/10 but not established for 0.0034.
2. **normalized-weight** — \(p_i = w_i / \sum w\).
3. **independent-rare-gate** — rares (labels or tiny weights) stay out of the common pool; commons normalize among themselves.

## Boost illustration (not proof)

Commons 90 and 10; Thorn target 23.5% via weight multiplier \(m\):  
\(10m/(90+10m)=0.235\) ⇒ \(m \approx 2.7647\) (not 2.35).

## Empirical vs published

51724-egg Secret Room interval: Agony 20/51724 ≈ 0.03867% (one in ~2586). Keep separate from the public `0.0034` field.
