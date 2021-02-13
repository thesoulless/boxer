package metrics_test

import (
	"testing"

	"github.com/thesoulless/boxer/metrics"

	"github.com/thesoulless/boxer"
)

func TestNew(t *testing.T) {
	type args struct {
		boxer     boxer.Boxer
		namespace string
		subSystem string
	}
	b, _ := boxer.New(false, "ns", "sub", "default")
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "first register",
			args: args{
				boxer:     b,
				namespace: "ns",
				subSystem: "sub",
			},
			wantErr: false,
		},
		{
			name: "collide",
			args: args{
				boxer:     b,
				namespace: "ns",
				subSystem: "sub",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := metrics.New(tt.args.boxer, tt.args.namespace, tt.args.subSystem); (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
