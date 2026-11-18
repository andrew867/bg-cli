package accountanalytics

import (
	"fmt"
	"path/filepath"

	"github.com/andrew867/bg-cli/internal/analysis"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

const reconcileMethodID = "accountanalytics.reconcile.v1"

// ReconcileOptions configures cross-view checks.
type ReconcileOptions struct {
	EggID              string
	RedactCounterparty bool
	RequireIntegrity   bool
	CatalogPets        []CatalogPet
}

// RunReconciliation executes integrity and cross-view checks, returning analysis.CheckResult entries.
func RunReconciliation(from, to *snapshot.Bundle, opt ReconcileOptions) []analysis.CheckResult {
	var out []analysis.CheckResult
	if from == nil || to == nil {
		out = append(out, analysis.CheckResult{
			ID: "bundle.present", Status: analysis.CheckFail, Detail: "from/to bundle required",
		})
		return out
	}

	// Manifest hashes / file integrity on "to" snapshot.
	if to.Manifest != nil && to.BaseDir != "" {
		manPath := to.Manifest.ManifestFile
		if !filepath.IsAbs(manPath) {
			manPath = filepath.Join(to.BaseDir, manPath)
		}
		rep, err := snapshot.ValidateIntegrity(manPath, opt.RequireIntegrity)
		if err != nil {
			out = append(out, analysis.CheckResult{
				ID: "manifest.integrity", Status: analysis.CheckInconclusive,
				Detail: err.Error(), Evidence: []string{reconcileMethodID},
			})
		} else if rep.OK {
			out = append(out, analysis.CheckResult{
				ID: "manifest.integrity", Status: analysis.CheckPass,
				Detail:   fmt.Sprintf("checked %d files", rep.Checked),
				Evidence: rep.Warnings,
			})
		} else {
			out = append(out, analysis.CheckResult{
				ID: "manifest.integrity", Status: analysis.CheckFail,
				Detail:   "digest/size mismatch",
				Evidence: rep.Failures,
			})
		}
	} else {
		out = append(out, analysis.CheckResult{
			ID: "manifest.integrity", Status: analysis.CheckSkipped,
			Detail: "no manifest path on bundle",
		})
	}

	// Egg vs hatch species reconciliation.
	if opt.EggID != "" && from.ProfileData != nil && to.ProfileData != nil {
		hr, err := HatchReport(from, to, opt.EggID)
		if err != nil {
			out = append(out, analysis.CheckResult{
				ID: "egg.hatch.species_sum", Status: analysis.CheckInconclusive, Detail: err.Error(),
			})
		} else if hr.FullyReconciled {
			out = append(out, analysis.CheckResult{
				ID: "egg.hatch.species_sum", Status: analysis.CheckPass,
				Detail:   fmt.Sprintf("species deltas %s match egg delta %s", hr.SpeciesDeltaSum, hr.EggPurchaseDelta),
				Evidence: []string{hr.MethodID},
			})
		} else {
			out = append(out, analysis.CheckResult{
				ID: "egg.hatch.species_sum", Status: analysis.CheckFail,
				Detail:   fmt.Sprintf("residual %s (egg %s, species %s)", hr.Residual, hr.EggPurchaseDelta, hr.SpeciesDeltaSum),
				Evidence: hr.Warnings,
			})
		}
	}

	// Inventory vs transactions (inconclusive when views missing).
	hasInv := from.Inventory != nil && to.Inventory != nil
	hasTx := hasTransactionViews(to)
	switch {
	case !hasInv || !hasTx:
		out = append(out, analysis.CheckResult{
			ID: "inventory.vs.transactions", Status: analysis.CheckInconclusive,
			Detail: "requires inventory on both snapshots and trades/booth/mail on at least the later snapshot",
		})
	default:
		fromPets, _ := snapshot.ExtractPetStacks(from.Inventory)
		toPets, _ := snapshot.ExtractPetStacks(to.Inventory)
		fromByID, _ := snapshot.AggregatePets(fromPets, "")
		toByID, _ := snapshot.AggregatePets(toPets, "")
		invDelta := snapshot.DiffExactMaps(toByID, fromByID)
		mov, err := ParseMovements(to)
		if err != nil {
			out = append(out, analysis.CheckResult{
				ID: "inventory.vs.transactions", Status: analysis.CheckInconclusive, Detail: err.Error(),
			})
		} else {
			sum := snapshot.ExactInt{Int: bigZero(), Text: "0", ParseOK: true, SourceText: "0"}
			for _, m := range mov {
				qty := snapshot.ParseExactIntString(m.Qty)
				if !qty.ParseOK {
					continue
				}
				switch m.Direction {
				case DirectionIn:
					sum = snapshot.AddExact(sum, qty)
				case DirectionOut:
					neg, _ := snapshot.SubExact(snapshot.ExactInt{Int: bigZero(), Text: "0", ParseOK: true, SourceText: "0"}, qty)
					sum = snapshot.AddExact(sum, neg)
				}
			}
			_ = invDelta
			out = append(out, analysis.CheckResult{
				ID: "inventory.vs.transactions", Status: analysis.CheckInconclusive,
				Detail:   "transaction views may be paginated; net movement sum is informational only",
				Evidence: []string{fmt.Sprintf("movements=%d netQty=%s", len(mov), sum.Text)},
			})
		}
	}

	// Index vs inventory (Pet class quantities).
	idxRaw, hasIdx := itemIndexRaw(to)
	if !hasIdx || to.Inventory == nil {
		out = append(out, analysis.CheckResult{
			ID: "index.vs.inventory", Status: analysis.CheckInconclusive,
			Detail: "requires itemIndex and inventory on later snapshot",
		})
	} else {
		parsed, err := ParseItemIndex(idxRaw)
		if err != nil {
			out = append(out, analysis.CheckResult{
				ID: "index.vs.inventory", Status: analysis.CheckInconclusive, Detail: err.Error(),
			})
		} else {
			petBucket := parsed.Classes["Pet"]
			stacks, _ := snapshot.ExtractPetStacks(to.Inventory)
			_, byVar := snapshot.AggregatePets(stacks, "")
			mismatch := 0
			if petBucket != nil {
				for _, vr := range petBucket.Variants {
					varQty := byVar[vr.BaseID][vr.Variant]
					if !varQty.ParseOK {
						mismatch++
						continue
					}
					if vr.CountExact.ParseOK && varQty.Text != vr.Count {
						mismatch++
					}
				}
			}
			if mismatch == 0 {
				out = append(out, analysis.CheckResult{
					ID: "index.vs.inventory", Status: analysis.CheckPass,
					Detail: "pet index variant totals align with owned inventory where comparable",
				})
			} else {
				out = append(out, analysis.CheckResult{
					ID: "index.vs.inventory", Status: analysis.CheckFail,
					Detail: fmt.Sprintf("%d variant count mismatches", mismatch),
				})
			}
		}
	}

	// Catalog completion note.
	if len(opt.CatalogPets) > 0 {
		if idxRaw, ok := itemIndexRaw(to); ok {
			if parsed, err := ParseItemIndex(idxRaw); err == nil {
				cr := CompletionReport(parsed, opt.CatalogPets)
				out = append(out, analysis.CheckResult{
					ID: "index.catalog.completion", Status: analysis.CheckPass,
					Detail: fmt.Sprintf("%.2f%% (%d/%d)", cr.Percent, cr.Discovered, cr.Denominator),
				})
			}
		}
	}

	// Currency cap notes from profile currency deltas.
	if from.ProfileData != nil && to.ProfileData != nil {
		fromPV, _ := snapshot.ExtractProfileView(from.ProfileData)
		toPV, _ := snapshot.ExtractProfileView(to.ProfileData)
		deltas := snapshot.DiffExactMaps(toPV.Currencies, fromPV.Currencies)
		for id, d := range deltas {
			start := fromPV.Currencies[id]
			end := toPV.Currencies[id]
			if snapshot.CurrencyAtKnownCap(id, start, end, d) {
				out = append(out, analysis.CheckResult{
					ID: "currency.cap." + id, Status: analysis.CheckInconclusive,
					Detail:   snapshot.CapFindingCapped,
					Evidence: []string{id},
				})
			}
		}
	}
	return out
}

func hasTransactionViews(b *snapshot.Bundle) bool {
	if b == nil || b.ViewRaw == nil {
		return false
	}
	for _, view := range []string{"trades", "booth", "mail"} {
		if raw, ok := b.ViewRaw[view]; ok && len(raw) > 0 {
			return true
		}
	}
	return false
}
