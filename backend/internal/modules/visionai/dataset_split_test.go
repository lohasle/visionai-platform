package visionai

import "testing"

func TestResolveDatasetSplitsSupportsRatioRulesAndExternalList(t *testing.T) {
	assets := []Asset{
		{ID: 101, BusinessScene: "day", Width: 1920, Metadata: `{"site":"A"}`},
		{ID: 102, BusinessScene: "night", Width: 1280, Metadata: `{"site":"B"}`},
	}
	ratios := map[string]float64{"TRAIN": 0.8, "VAL": 0.1, "TEST": 0.1}

	ratio, config, err := resolveDatasetSplits(datasetVersionRequest{
		SplitMode: "RATIO", SplitSeed: 42,
	}, assets, ratios)
	if err != nil || len(ratio) != 2 || config == nil {
		t.Fatalf("ratio split failed: assignments=%#v config=%#v err=%v", ratio, config, err)
	}
	repeated, _, _ := resolveDatasetSplits(datasetVersionRequest{
		SplitMode: "RATIO", SplitSeed: 42,
	}, assets, ratios)
	if ratio[101] != repeated[101] || ratio[102] != repeated[102] {
		t.Fatal("fixed-seed ratio split is not reproducible")
	}

	rules, _, err := resolveDatasetSplits(datasetVersionRequest{
		SplitMode: "RULE",
		SplitRules: []datasetSplitRule{
			{Field: "businessScene", Operator: "EQ", Value: "night", Split: "TEST"},
			{Field: "metadata.site", Operator: "EQ", Value: "A", Split: "TRAIN"},
		},
	}, assets, ratios)
	if err != nil || rules[101] != "TRAIN" || rules[102] != "TEST" {
		t.Fatalf("rule split failed: assignments=%#v err=%v", rules, err)
	}

	external, _, err := resolveDatasetSplits(datasetVersionRequest{
		SplitMode:           "EXTERNAL_LIST",
		ExternalAssignments: map[string]string{"101": "VAL", "102": "TEST"},
	}, assets, ratios)
	if err != nil || external[101] != "VAL" || external[102] != "TEST" {
		t.Fatalf("external split failed: assignments=%#v err=%v", external, err)
	}
}

func TestResolveDatasetSplitsRejectsIncompleteRulesAndExternalList(t *testing.T) {
	assets := []Asset{{ID: 101, BusinessScene: "day", Metadata: `{}`}}
	ratios := map[string]float64{"TRAIN": 0.8, "VAL": 0.1, "TEST": 0.1}
	if _, _, err := resolveDatasetSplits(datasetVersionRequest{
		SplitMode: "RULE",
		SplitRules: []datasetSplitRule{
			{Field: "businessScene", Operator: "EQ", Value: "night", Split: "TEST"},
		},
	}, assets, ratios); err == nil {
		t.Fatal("an asset that misses every split rule must be rejected")
	}
	if _, _, err := resolveDatasetSplits(datasetVersionRequest{
		SplitMode: "EXTERNAL_LIST", ExternalAssignments: map[string]string{},
	}, assets, ratios); err == nil {
		t.Fatal("an incomplete external assignment list must be rejected")
	}
}
