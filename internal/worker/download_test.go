package worker

import (
	"context"
	"testing"

	"github.com/mononen/stasharr/internal/config"
)

func TestIsRedirectURL(t *testing.T) {
	tests := []struct {
		name        string
		prowlarrURL string
		downloadURL string
		want        bool
	}{
		{
			name:        "prowlarr proxy URL (redirect off)",
			prowlarrURL: "http://prowlarr:9696",
			downloadURL: "http://prowlarr:9696/api/v1/download?apikey=abc&id=123",
			want:        false,
		},
		{
			name:        "direct indexer URL (redirect on)",
			prowlarrURL: "http://prowlarr:9696",
			downloadURL: "https://strict-indexer.example.com/download?apikey=secret&id=456",
			want:        true,
		},
		{
			name:        "prowlarr on custom domain",
			prowlarrURL: "https://prowlarr.mydomain.com",
			downloadURL: "https://prowlarr.mydomain.com/api/v1/download?apikey=abc",
			want:        false,
		},
		{
			name:        "indexer when prowlarr on custom domain",
			prowlarrURL: "https://prowlarr.mydomain.com",
			downloadURL: "https://indexer.example.com/dl?apikey=secret",
			want:        true,
		},
		{
			name:        "prowlarr URL with path prefix",
			prowlarrURL: "http://192.168.1.10:9696",
			downloadURL: "http://192.168.1.10:9696/api/v1/download?apikey=abc",
			want:        false,
		},
		{
			name:        "same hostname different port is a redirect",
			prowlarrURL: "http://prowlarr:9696",
			downloadURL: "http://prowlarr:8080/download?apikey=abc",
			want:        true,
		},
		{
			name:        "unparseable prowlarr URL falls back to non-redirect",
			prowlarrURL: "://bad url",
			downloadURL: "https://indexer.example.com/dl",
			want:        false,
		},
		{
			name:        "unparseable download URL returns false",
			prowlarrURL: "http://prowlarr:9696",
			downloadURL: "://bad url",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := config.LoadFromDB(context.Background(), nil)
			cfg.Set("prowlarr.url", tt.prowlarrURL)

			w := &DownloadWorker{
				Base: Base{config: cfg},
			}

			got := w.isRedirectURL(tt.downloadURL)
			if got != tt.want {
				t.Errorf("isRedirectURL(%q) with prowlarr=%q = %v, want %v",
					tt.downloadURL, tt.prowlarrURL, got, tt.want)
			}
		})
	}
}
