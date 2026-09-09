package directives

import (
	"encoding/json"
	"os"
	"testing"
)

func TestHasCodeownersApproval(t *testing.T) {
	data, err := os.ReadFile("testdata/codeowners_approved.json")
	if err != nil {
		t.Fatal(err)
	}
	var tt []struct {
		Name     string `json:"name"`
		Body     string `json:"body"`
		Expected bool   `json:"expected"`
	}
	if err := json.Unmarshal(data, &tt); err != nil {
		t.Fatal(err)
	}
	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			if actual := HasCodeownersApproval(tc.Body); actual != tc.Expected {
				t.Errorf("HasCodeownersApproval(%q) = %v, expected %v", tc.Body, actual, tc.Expected)
			}
		})
	}
}
