package save

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/pidgy/unitehud/core/stats"
	"github.com/pidgy/unitehud/exe"
)

func TestTemplateStatistics(t *testing.T) {
	exe.Debug = true
	templateStatistics(stats.Counts())
}

func TestMergeTemplateStatistics(t *testing.T) {
	all := make(map[string]int)

	b, err := os.ReadFile("../../saved/templates.json")
	if err != nil {
		if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	if len(b) == 0 {
		b = []byte("{}")
	}

	err = json.Unmarshal(b, &all)
	if err != nil {
		t.Fatal(err)
	}

	new := make(map[string]int)

	for k, v := range all {
		args := strings.Split(k, "device/")
		if len(args) > 1 {
			k = args[1]
		}

		new[k] += v
	}

	b, err = json.Marshal(new)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile("test_merge_templates.json", sortedJSON(b), os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}
}
