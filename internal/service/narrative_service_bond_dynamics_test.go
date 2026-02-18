package service

import (
	"strings"
	"testing"
)

func TestDeriveBondDynamicsThresholdsAndModes(t *testing.T) {
	tests := []struct {
		name     string
		trust    int
		intimacy int
		respect  int
		mustHave []string
		mustNot  []string
	}{
		{
			name:     "jealous mode at threshold",
			trust:    40,
			intimacy: 70,
			respect:  50,
			mustHave: []string{"MODO: CELOS PATOLOGICOS", "maximo 1 pregunta"},
		},
		{
			name:     "hostility mode at threshold",
			trust:    80,
			intimacy: 30,
			respect:  35,
			mustHave: []string{"MODO: HOSTILIDAD DESPECTIVA"},
		},
		{
			name:     "both modes when both conditions match",
			trust:    10,
			intimacy: 95,
			respect:  10,
			mustHave: []string{"MODO: CELOS PATOLOGICOS", "MODO: HOSTILIDAD DESPECTIVA"},
		},
		{
			name:     "neutral when no mode applies",
			trust:    41,
			intimacy: 69,
			respect:  36,
			mustHave: []string{"vinculo relativamente estable/neutral"},
			mustNot:  []string{"MODO:"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveBondDynamics(tc.trust, tc.intimacy, tc.respect)
			for _, s := range tc.mustHave {
				if !strings.Contains(got, s) {
					t.Fatalf("deriveBondDynamics(%d,%d,%d) missing %q in %q", tc.trust, tc.intimacy, tc.respect, s, got)
				}
			}
			for _, s := range tc.mustNot {
				if strings.Contains(got, s) {
					t.Fatalf("deriveBondDynamics(%d,%d,%d) should not contain %q in %q", tc.trust, tc.intimacy, tc.respect, s, got)
				}
			}
		})
	}
}
