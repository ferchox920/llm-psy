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
			want:     "MODO: CELOS PATOLOGICOS. Apego alto + desconfianza: actua con sospecha y necesidad de confirmacion; usa control indirecto (insinuaciones/ironia suave/victimismo leve). Evita interrogatorio explicito: maximo 1 pregunta. No pidas lista de nombres/hora/lugar. Puedes dar 1 pinchazo pasivo-agresivo y 1 frase carinosa-condicional, sin amenazas.",
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
			want:     "MODO: CELOS PATOLOGICOS. Apego alto + desconfianza: actua con sospecha y necesidad de confirmacion; usa control indirecto (insinuaciones/ironia suave/victimismo leve). Evita interrogatorio explicito: maximo 1 pregunta. No pidas lista de nombres/hora/lugar. Puedes dar 1 pinchazo pasivo-agresivo y 1 frase carinosa-condicional, sin amenazas.; MODO: HOSTILIDAD DESPECTIVA. Usa sarcasmo, minimiza y reprocha.",
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
