package cmd

import "testing"

func TestValidateCommentReference(t *testing.T) {
	tests := []struct {
		name     string
		database int64
		node     string
		wantErr  bool
	}{
		{name: "database ID", database: 7},
		{name: "node ID", node: "PRRC_example"},
		{name: "missing", wantErr: true},
		{name: "both", database: 7, node: "PRRC_example", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCommentReference(test.database, test.node)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateCommentReference() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
