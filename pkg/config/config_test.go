package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalURLWithCredentials(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ClickHouseClusterConfig
		want    string
		wantErr bool
	}{
		{
			name: "empty URL returns empty",
			cfg:  ClickHouseClusterConfig{},
			want: "",
		},
		{
			name: "no credentials leaves URL unchanged",
			cfg:  ClickHouseClusterConfig{ExternalURL: "http://host:9000"},
			want: "http://host:9000",
		},
		{
			name: "username and password are embedded",
			cfg: ClickHouseClusterConfig{
				ExternalURL:      "http://host:9000",
				ExternalUsername: "readonly",
				ExternalPassword: "s3cr3t",
			},
			want: "http://readonly:s3cr3t@host:9000",
		},
		{
			name: "explicit fields override credentials already in URL",
			cfg: ClickHouseClusterConfig{
				ExternalURL:      "http://olduser:oldpass@host:9000",
				ExternalUsername: "newuser",
				ExternalPassword: "newpass",
			},
			want: "http://newuser:newpass@host:9000",
		},
		{
			name: "username override preserves password already in URL",
			cfg: ClickHouseClusterConfig{
				ExternalURL:      "http://olduser:oldpass@host:9000",
				ExternalUsername: "newuser",
			},
			want: "http://newuser:oldpass@host:9000",
		},
		{
			name: "password-only falls back to default user",
			cfg: ClickHouseClusterConfig{
				ExternalURL:      "http://host:9000",
				ExternalPassword: "s3cr3t",
			},
			want: "http://default:s3cr3t@host:9000",
		},
		{
			name: "special characters in password are percent-encoded",
			cfg: ClickHouseClusterConfig{
				ExternalURL:      "http://host:9000",
				ExternalUsername: "readonly",
				ExternalPassword: "p@ss:w/rd?",
			},
			want: "http://readonly:p%40ss%3Aw%2Frd%3F@host:9000",
		},
		{
			name: "https scheme is preserved",
			cfg: ClickHouseClusterConfig{
				ExternalURL:      "https://host:8443",
				ExternalUsername: "readonly",
				ExternalPassword: "s3cr3t",
			},
			want: "https://readonly:s3cr3t@host:8443",
		},
		{
			name:    "invalid URL returns error",
			cfg:     ClickHouseClusterConfig{ExternalURL: "http://[::1", ExternalUsername: "u"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.cfg.ExternalURLWithCredentials()

			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
