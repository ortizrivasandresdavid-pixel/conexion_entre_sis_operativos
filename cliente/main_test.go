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
			name:    "rechaza ausencia de IP",
			args:    nil,
			wantErr: true,
		},
		{
			name:        "IP sola usa valores predeterminados",
			args:        []string{"10.8.0.1"},
			wantAddress: "10.8.0.1:2306",
			wantPeriod:  5,
		},
		{
			name:        "acepta puerto e intervalo opcionales",
			args:        []string{"2001:db8::1", "2400", "10"},
			wantAddress: "[2001:db8::1]:2400",
			wantPeriod:  10,
		},
		{
			name:    "rechaza IP inválida",
			args:    []string{"servidor.local"},
			wantErr: true,
		},
		{
			name:    "rechaza puerto inválido",
			args:    []string{"10.8.0.1", "0"},
			wantErr: true,
		},
		{
			name:    "rechaza intervalo inválido",
			args:    []string{"10.8.0.1", "2306", "0"},
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
