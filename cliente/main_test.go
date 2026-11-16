package main

import "testing"

func TestParseClientArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantAddress string
		wantPeriod  int
		wantErr     bool
	}{
		{
			name:    "rechaza ausencia de argumentos",
			args:    nil,
			wantErr: true,
		},
		{
			name:    "rechaza IP sin puerto ni período",
			args:    []string{"10.8.0.1"},
			wantErr: true,
		},
		{
			name:    "rechaza IP y puerto sin período",
			args:    []string{"10.8.0.1", "2306"},
			wantErr: true,
		},
		{
			name:        "acepta IP, puerto y período",
			args:        []string{"2001:db8::1", "2400", "10"},
			wantAddress: "[2001:db8::1]:2400",
			wantPeriod:  10,
		},
		{
			name:    "rechaza IP inválida",
			args:    []string{"servidor.local", "2306", "5"},
			wantErr: true,
		},
		{
			name:    "rechaza puerto cero",
			args:    []string{"10.8.0.1", "0", "5"},
			wantErr: true,
		},
		{
			name:    "rechaza puerto no numérico",
			args:    []string{"10.8.0.1", "abc", "5"},
			wantErr: true,
		},
		{
			name:    "rechaza período cero",
			args:    []string{"10.8.0.1", "2306", "0"},
			wantErr: true,
		},
		{
			name:    "rechaza período negativo",
			args:    []string{"10.8.0.1", "2306", "-5"},
			wantErr: true,
		},
		{
			name:    "rechaza período no numérico",
			args:    []string{"10.8.0.1", "2306", "abc"},
			wantErr: true,
		},
		{
			name:    "rechaza período fuera del máximo",
			args:    []string{"10.8.0.1", "2306", "3601"},
			wantErr: true,
		},
		{
			name:    "rechaza argumentos adicionales",
			args:    []string{"10.8.0.1", "2306", "5", "extra"},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			address, period, err := parseClientArgs(test.args)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseClientArgs() error = %v, wantErr %v", err, test.wantErr)
			}
			if err != nil {
				return
			}
			if address != test.wantAddress {
				t.Errorf("address = %q, want %q", address, test.wantAddress)
			}
			if period != test.wantPeriod {
				t.Errorf("period = %d, want %d", period, test.wantPeriod)
			}
		})
	}
}
