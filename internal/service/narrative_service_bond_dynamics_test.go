package service

import "testing"

func TestDeriveBondDynamics(t *testing.T) {
	tests := []struct {
		name     string
		trust    int
		intimacy int
		respect  int
		want     string
	}{
		{
			name:     "amor toxico: intimidad alta + confianza baja => celos/control",
			trust:    10,
			intimacy: 90,
			respect:  50,
			want:     "MODO: CELOS PATOLOGICOS. Actua desde desconfianza activa, posesividad y necesidad de control; busca subtexto, cuestiona y usa ironia.",
		},
		{
			name:     "respeto bajo => reproches/hostilidad",
			trust:    60,
			intimacy: 40,
			respect:  30,
			want:     "MODO: HOSTILIDAD DESPECTIVA. Usa sarcasmo, minimiza y reprocha.",
		},
		{
			name:     "ambas reglas aplican => ambas frases",
			trust:    20,
			intimacy: 80,
			respect:  10,
			want:     "MODO: CELOS PATOLOGICOS. Actua desde desconfianza activa, posesividad y necesidad de control; busca subtexto, cuestiona y usa ironia.; MODO: HOSTILIDAD DESPECTIVA. Usa sarcasmo, minimiza y reprocha.",
		},
		{
			name:     "fallback neutral",
			trust:    50,
			intimacy: 50,
			respect:  50,
			want:     "vinculo relativamente estable/neutral",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveBondDynamics(tt.trust, tt.intimacy, tt.respect)
			if got != tt.want {
				t.Fatalf("deriveBondDynamics() = %q, want %q", got, tt.want)
			}
		})
	}
}
