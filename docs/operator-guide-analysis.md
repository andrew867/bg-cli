# Operator guide — snapshots, loadouts, and Forge

Concise end-to-end flow for read-only account analysis.

## 1. Auth and doctor

```powershell
bg-cli auth login --profile main --client-id YOUR_ID --redirect-uri http://127.0.0.1:8765/callback
bg-cli doctor
```

## 2. Snapshots and offline diffs

```powershell
bg-cli account snapshot --profile main --dir . --prefix ps99 --note "baseline"
bg-cli account snapshot diff --from OLD-manifest.json --to NEW-manifest.json --egg "Masquerade Egg" --pet "Butler Penguin" --pretty
```

## 3. Loadout and pet power

```powershell
bg-cli account loadout analyze --manifest .\*-manifest.json --world halloween-games --pretty
bg-cli account power inspect --snapshot .\*-manifest.json --world halloween-games --annotations .\annotations.json --pretty
bg-cli account loadout recommend --snapshot .\*-manifest.json --objective currency-rate --currency HalloweenGamesCoins --event-team-limit 15 --pretty
```

Use `slot` / `instanceKey` in annotations — equipped `uid` can be shared stack identity.

## 4. Experiments / calibration

```powershell
bg-cli account experiment compare --from BEFORE-manifest.json --to AFTER-manifest.json --currency HalloweenGamesCoins --pretty
```

## 5. Halloween Forge (offline)

```powershell
bg-cli account forge observe --file .\forge-observations.json --pretty
bg-cli account forge analyze --observations .\forge-observations.json --pretty
bg-cli account forge optimize --observations .\forge-observations.json --snapshot .\*-manifest.json --objective min-butlers --pretty
bg-cli account forge ledger --file .\forge-attempts.json --pretty
```

Displayed Forge probabilities are UI measurements. Recommendations never execute a Forge.
