# Public data CLI

```powershell
bg-cli public discover --fixture .\testdata\fixtures\observatory\collections-list.json --pretty
bg-cli public sync --collections Eggs,Pets --dir .\.bg-cli-catalog --pretty
bg-cli public status --dir .\.bg-cli-catalog --pretty
bg-cli public inspect Eggs "Secret Room Egg" --dir .\.bg-cli-catalog --pretty

bg-cli egg show "Secret Room Egg" --file .\testdata\fixtures\observatory\eggs-secret-room.json --pretty
bg-cli egg odds "Secret Room Egg" --model normalized-weight --rolls 51724 --pretty
bg-cli egg odds "Secret Room Egg" --model independent-rare-gate --rolls 51724 --pretty
bg-cli egg flags "Secret Room Egg" --pretty
bg-cli egg cost "Secret Room Egg" --balance 1118683238213 --batch-size 67 --batch-cost 1440000000 --pretty

bg-cli graph unresolved --pretty
```

Catalog sync never attaches account tokens and must not spend inventory refresh quota.
