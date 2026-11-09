package version

import "testing"

func TestInfo_String(t *testing.T) {
	tests := []struct {
		name string
		info Info
		want string
	}{
		{
			name: "with version and commit",
			info: Info{Version: "1.0.0", Commit: "abc123"},
			want: "skill-manager-cli version 1.0.0 (commit abc123)",
		},
		{
			name: "with defaults",
			info: Info{Version: "dev", Commit: "unknown"},
			want: "skill-manager-cli version dev (commit unknown)",
		},
		{
			name: "empty commit",
			info: Info{Version: "1.0.0", Commit: ""},
			want: "skill-manager-cli version 1.0.0 (commit )",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.String(); got != tt.want {
				t.Errorf("Info.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
