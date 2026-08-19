package validation_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"fake-mex-backend/internal/validation"
)

type decimalFixture struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Scale   int    `json:"scale"`
	WantErr bool   `json:"wantErr"`
}

func TestValidateDecimalFixtureDriven(t *testing.T) {
	t.Parallel()

	fixturePath := filepath.Join("testdata", "decimals.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var fixtures []decimalFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	for _, f := range fixtures {
		fixture := f
		t.Run(fixture.Name, func(t *testing.T) {
			t.Parallel()
			err := validation.ValidateDecimal(fixture.Value)
			if err == nil && fixture.WantErr {
				t.Fatalf("expected error for value=%q", fixture.Value)
			}
			if err != nil && !fixture.WantErr {
				t.Fatalf("unexpected error for value=%q: %v", fixture.Value, err)
			}
		})
	}
}

func TestValidateScaleFixtureDriven(t *testing.T) {
	t.Parallel()

	type scaleFixture struct {
		Name    string `json:"name"`
		Value   string `json:"value"`
		Scale   int    `json:"scale"`
		WantErr bool   `json:"wantErr"`
	}

	data, err := os.ReadFile(filepath.Join("testdata", "scale_cases.json"))
	if err != nil {
		// Keep test fixtures focused on one source of truth; if this file is
		// missing the test should fail loudly.
		t.Fatalf("read fixture: %v", err)
	}

	var fixtures []scaleFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	for _, tc := range fixtures {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			if err := validation.ValidateScale(tc.Value, tc.Scale); (err != nil) != tc.WantErr {
				t.Fatalf("value=%q scale=%d got err=%v wantErr=%v", tc.Value, tc.Scale, err, tc.WantErr)
			}
		})
	}
}
