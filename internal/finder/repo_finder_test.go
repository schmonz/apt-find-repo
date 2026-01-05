package finder

import (
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

type TestCase struct {
	name        string
	htmlFile    string
	expectedGPG []string
	expectedDeb []string
}

var testCases = []TestCase{
	{
		name:     "zoom-unofficial",
		htmlFile: "../../testdata/webpages/zoom-unofficial.html",
		expectedGPG: []string{
			"https://mirror.mwt.me/zoom/gpgkey",
		},
		expectedDeb: []string{
			"deb [arch=amd64 signed-by=/etc/apt/keyrings/mwt.asc by-hash=force] https://mirror.mwt.me/zoom/deb any main",
			"deb [arch=amd64 signed-by=/etc/apt/keyrings/mwt.asc by-hash=force] https://mirror.mwt.me/rstudio/deb/jammy jammy main",
		},
	},
	{
		name:     "jetbrains-unofficial",
		htmlFile: "../../testdata/webpages/jetbrains-unofficial.html",
		expectedGPG: []string{
			"https://s3.eu-central-1.amazonaws.com/jetbrains-ppa/0xA6E8698A.pub.asc",
		},
		expectedDeb: []string{
			"deb [signed-by=/usr/share/keyrings/jetbrains-ppa-archive-keyring.gpg] http://jetbrains-ppa.s3-website.eu-central-1.amazonaws.com any main",
		},
	},
	{
		name:     "tailscale-official",
		htmlFile: "../../testdata/webpages/tailscale-official.html",
		expectedGPG: []string{
			"https://pkgs.tailscale.com/stable/ubuntu/jammy.noarmor.gpg",
			"https://pkgs.tailscale.com/stable/ubuntu/focal.noarmor.gpg",
		},
		expectedDeb: []string{
			"deb [signed-by=/usr/share/keyrings/tailscale-archive-keyring.gpg] https://pkgs.tailscale.com/stable/ubuntu jammy main",
			"deb [signed-by=/usr/share/keyrings/tailscale-archive-keyring.gpg] https://pkgs.tailscale.com/stable/ubuntu focal main",
		},
	},
	{
		name:     "postgresql-official",
		htmlFile: "../../testdata/webpages/postgresql-official.html",
		expectedGPG: []string{
			"https://www.postgresql.org/media/keys/ACCC4CF8.asc",
		},
		expectedDeb: []string{},
	},
	{
		name:     "nginx-official",
		htmlFile: "../../testdata/webpages/nginx-official.html",
		expectedGPG: []string{
			"https://nginx.org/keys/nginx_signing.key",
		},
		expectedDeb: []string{},
	},
	{
		name:     "sublime-official",
		htmlFile: "../../testdata/webpages/sublime-official.html",
		expectedGPG: []string{
			"https://download.sublimetext.com/sublimehq-pub.gpg",
			"https://download.sublimetext.com/sublimehq-rpm-pub.gpg",
		},
		expectedDeb: []string{},
	},
	{
		name:     "signal-official",
		htmlFile: "../../testdata/webpages/signal-official.html",
		expectedGPG: []string{
			"https://updates.signal.org/desktop/apt/keys.asc",
		},
		expectedDeb: []string{},
	},
	{
		name:     "grafana-official",
		htmlFile: "../../testdata/webpages/grafana-official.html",
		expectedGPG: []string{
			"https://apt.grafana.com/gpg.key",
			"https://apt.grafana.com/gpg",
		},
		expectedDeb: []string{
			"deb [signed-by=/etc/apt/keyrings/grafana.gpg] https://apt.grafana.com stable main",
			"deb [signed-by=/etc/apt/keyrings/grafana.gpg] https://apt.grafana.com beta main",
		},
	},
	{
		name:     "github-cli-official",
		htmlFile: "../../testdata/webpages/github-cli-official.html",
		expectedGPG: []string{
			"https://cli.github.com/packages/githubcli-archive-keyring.gpg",
		},
		expectedDeb: []string{
			"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main",
			`deb [arch=\u003cspan class=\"pl-s\"\u003e\u003cspan class=\"pl-pds\"\u003e$(\u003c/span\u003edpkg --print-architecture\u003cspan class=\"pl-pds\"\u003e)\u003c/span\u003e\u003c/span\u003e signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main`,
		},
	},
	{
		name:     "syncthing-official",
		htmlFile: "../../testdata/webpages/syncthing-official.html",
		expectedGPG: []string{
			"https://syncthing.net/release-key.gpg",
		},
		expectedDeb: []string{
			"deb [signed-by=/etc/apt/keyrings/syncthing-archive-keyring.gpg] https://apt.syncthing.net/ syncthing stable",
			"deb [signed-by=/etc/apt/keyrings/syncthing-archive-keyring.gpg] https://apt.syncthing.net/ syncthing candidate",
		},
	},
	{
		name:     "kubernetes-official",
		htmlFile: "../../testdata/webpages/kubernetes-official.html",
		expectedGPG: []string{
			"https://pkgs.k8s.io/core:/stable:/v1.35/deb/Release.key",
			"https://pkgs.k8s.io/core:/stable:/v1.35/rpm/repodata/repomd.xml.key",
		},
		expectedDeb: []string{},
	},
	{
		name:     "1password-official",
		htmlFile: "../../testdata/webpages/1password-official.html",
		expectedGPG: []string{
			"https://downloads.1password.com/linux/keys/1password.asc",
		},
		expectedDeb: []string{
			"deb [arch=amd64 signed-by=/usr/share/keyrings/1password-archive-keyring.gpg] https://downloads.1password.com/linux/debian/amd64 stable main",
		},
	},
	{
		name:     "headscale-unofficial",
		htmlFile: "../../testdata/webpages/headscale-unofficial.html",
		expectedGPG: []string{
			"https://allddd.github.io/headscale-apt/headscale-apt.gpg",
		},
		expectedDeb: []string{
			"deb [arch=amd64 signed-by=/etc/apt/keyrings/headscale-apt.gpg] https://allddd.github.io/headscale-apt/ stable main",
		},
	},
	{
		name:        "hugo-unofficial",
		htmlFile:    "../../testdata/webpages/hugo-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/hugo.gpg] https://hugo-apt.8hob.io latest main",
		},
	},
	{
		name:        "ghostty-unofficial",
		htmlFile:    "../../testdata/webpages/ghostty-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{},
	},
	{
		name:        "audacity-unofficial",
		htmlFile:    "../../testdata/webpages/audacity-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb https://ppa.launchpadcontent.net/audacity-team/daily/ubuntu YOUR_UBUNTU_VERSION_HERE main deb-src https",
		},
	},
	{
		name:     "brave-official",
		htmlFile: "../../testdata/webpages/brave-official.html",
		expectedGPG: []string{
			"https://brave-browser-apt-release.s3.brave.com/brave-browser-archive-keyring.gpg",
			"https://brave-browser-apt-beta.s3.brave.com/brave-browser-beta-archive-keyring.gpg",
			"https://brave-browser-apt-nightly.s3.brave.com/brave-browser-nightly-archive-keyring.gpg",
		},
		expectedDeb: []string{},
	},
	{
		name:     "caddy-official",
		htmlFile: "../../testdata/webpages/caddy-official.html",
		expectedGPG: []string{
			"https://dl.cloudsmith.io/public/caddy/stable/gpg.key",
			"https://dl.cloudsmith.io/public/caddy/testing/gpg.key",
			"https://dl.cloudsmith.io/public/caddy/stable/gpg",
			"https://dl.cloudsmith.io/public/caddy/testing/gpg",
		},
		expectedDeb: []string{},
	},
	{
		name:        "chrome-official",
		htmlFile:    "../../testdata/webpages/chrome-official.html",
		expectedGPG: []string{},
		expectedDeb: []string{},
	},
	{
		name:     "docker-official",
		htmlFile: "../../testdata/webpages/docker-official.html",
		expectedGPG: []string{
			"https://download.docker.com/linux/ubuntu/gpg",
		},
		expectedDeb: []string{},
	},
	{
		name:        "elasticsearch-official",
		htmlFile:    "../../testdata/webpages/elasticsearch-official.html",
		expectedGPG: []string{},
		expectedDeb: []string{},
	},
	{
		name:        "gitlab-runner-official",
		htmlFile:    "../../testdata/webpages/gitlab-runner-official.html",
		expectedGPG: []string{},
		expectedDeb: []string{},
	},
	{
		name:        "graphics-drivers-unofficial",
		htmlFile:    "../../testdata/webpages/graphics-drivers-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb https://ppa.launchpadcontent.net/graphics-drivers/ppa/ubuntu YOUR_UBUNTU_VERSION_HERE main deb-src https",
		},
	},
	{
		name:        "handbrake-unofficial",
		htmlFile:    "../../testdata/webpages/handbrake-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb https://ppa.launchpadcontent.net/savoury1/handbrake/ubuntu YOUR_UBUNTU_VERSION_HERE main deb-src https",
		},
	},
	{
		name:        "influxdb-official",
		htmlFile:    "../../testdata/webpages/influxdb-official.html",
		expectedGPG: []string{},
		expectedDeb: []string{},
	},
	{
		name:        "inkscape-unofficial",
		htmlFile:    "../../testdata/webpages/inkscape-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb https://ppa.launchpadcontent.net/inkscape.dev/stable/ubuntu YOUR_UBUNTU_VERSION_HERE main deb-src https",
		},
	},
	{
		name:        "kdenlive-unofficial",
		htmlFile:    "../../testdata/webpages/kdenlive-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb https://ppa.launchpadcontent.net/kdenlive/kdenlive-stable/ubuntu YOUR_UBUNTU_VERSION_HERE main deb-src https",
		},
	},
	{
		name:        "libreoffice-unofficial",
		htmlFile:    "../../testdata/webpages/libreoffice-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb https://ppa.launchpadcontent.net/libreoffice/ppa/ubuntu YOUR_UBUNTU_VERSION_HERE main deb-src https",
		},
	},
	{
		name:        "lutris-unofficial",
		htmlFile:    "../../testdata/webpages/lutris-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb https://ppa.launchpadcontent.net/lutris-team/lutris/ubuntu YOUR_UBUNTU_VERSION_HERE main deb-src https",
		},
	},
	{
		name:        "mongodb-official",
		htmlFile:    "../../testdata/webpages/mongodb-official.html",
		expectedGPG: []string{},
		expectedDeb: []string{},
	},
	{
		name:        "neovim-unofficial",
		htmlFile:    "../../testdata/webpages/neovim-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb https://ppa.launchpadcontent.net/neovim-ppa/stable/ubuntu YOUR_UBUNTU_VERSION_HERE main deb-src https",
		},
	},
	{
		name:        "nodejs-unofficial",
		htmlFile:    "../../testdata/webpages/nodejs-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{},
	},
	{
		name:        "obs-studio-unofficial",
		htmlFile:    "../../testdata/webpages/obs-studio-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb https://ppa.launchpadcontent.net/obsproject/obs-studio/ubuntu YOUR_UBUNTU_VERSION_HERE main deb-src https",
		},
	},
	{
		name:        "oibaf-graphics-unofficial",
		htmlFile:    "../../testdata/webpages/oibaf-graphics-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb https://ppa.launchpadcontent.net/oibaf/graphics-drivers/ubuntu YOUR_UBUNTU_VERSION_HERE main deb-src https",
		},
	},
	{
		name:        "openshot-unofficial",
		htmlFile:    "../../testdata/webpages/openshot-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb https://ppa.launchpadcontent.net/openshot.developers/ppa/ubuntu YOUR_UBUNTU_VERSION_HERE main deb-src https",
		},
	},
	{
		name:        "redis-official",
		htmlFile:    "../../testdata/webpages/redis-official.html",
		expectedGPG: []string{},
		expectedDeb: []string{},
	},
	{
		name:        "savoury-ffmpeg-unofficial",
		htmlFile:    "../../testdata/webpages/savoury-ffmpeg-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb https://ppa.launchpadcontent.net/savoury1/ffmpeg5/ubuntu YOUR_UBUNTU_VERSION_HERE main deb-src https",
		},
	},
	{
		name:        "savoury-multimedia-unofficial",
		htmlFile:    "../../testdata/webpages/savoury-multimedia-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb https://ppa.launchpadcontent.net/savoury1/multimedia/ubuntu YOUR_UBUNTU_VERSION_HERE main deb-src https",
		},
	},
	{
		name:        "slack-official",
		htmlFile:    "../../testdata/webpages/slack-official.html",
		expectedGPG: []string{},
		expectedDeb: []string{},
	},
	{
		name:        "spotify-official",
		htmlFile:    "../../testdata/webpages/spotify-official.html",
		expectedGPG: []string{},
		expectedDeb: []string{},
	},
	{
		name:     "terraform-official",
		htmlFile: "../../testdata/webpages/terraform-official.html",
		expectedGPG: []string{
			"https://apt.releases.hashicorp.com/gpg",
		},
		expectedDeb: []string{},
	},
	{
		name:        "ubuntuhandbook-ffmpeg-unofficial",
		htmlFile:    "../../testdata/webpages/ubuntuhandbook-ffmpeg-unofficial.html",
		expectedGPG: []string{},
		expectedDeb: []string{
			"deb https://ppa.launchpadcontent.net/ubuntuhandbook1/ffmpeg8/ubuntu YOUR_UBUNTU_VERSION_HERE main deb-src https",
		},
	},
	{
		name:     "vscode-official",
		htmlFile: "../../testdata/webpages/vscode-official.html",
		expectedGPG: []string{
			"https://packages.microsoft.com/keys/microsoft.asc",
		},
		expectedDeb: []string{},
	},
	{
		name:        "yarn-official",
		htmlFile:    "../../testdata/webpages/yarn-official.html",
		expectedGPG: []string{},
		expectedDeb: []string{},
	},
}

func TestParseRepoPage(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			html, err := os.ReadFile(tc.htmlFile)
			if err != nil {
				t.Fatalf("Failed to read test file %s: %v", tc.htmlFile, err)
			}

			gpgURLs, debLines := ParseRepoPage(string(html))

			// Check GPG keys
			if !stringSlicesEqual(gpgURLs, tc.expectedGPG) {
				t.Errorf("GPG keys mismatch.\nGot:      %v\nExpected: %v", gpgURLs, tc.expectedGPG)
			}

			// Check deb lines
			if !stringSlicesEqual(debLines, tc.expectedDeb) {
				t.Errorf("Deb lines mismatch.\nGot:      %v\nExpected: %v", debLines, tc.expectedDeb)
			}
		})
	}
}

func TestFindDebLines(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		wantDebs []string
	}{
		{
			name:     "simple-code-block",
			html:     `<pre><code>deb https://example.com/repo stable main</code></pre>`,
			wantDebs: []string{"deb https://example.com/repo stable main"},
		},
		{
			name:     "with-echo-command",
			html:     `<code>echo "deb https://example.com/repo stable main" | sudo tee /etc/apt/sources.list.d/repo.list</code>`,
			wantDebs: []string{"deb https://example.com/repo stable main"},
		},
		{
			name: "multiple-code-blocks-same-deb",
			html: `
				<code>echo "deb https://example.com/repo stable main" | sudo tee /etc/apt/sources.list.d/repo.list</code>
				<p>Then run:</p>
				<code>deb https://example.com/repo stable main</code>
			`,
			wantDebs: []string{"deb https://example.com/repo stable main"},
		},
		{
			name: "different-repos",
			html: `
				<code>deb https://repo-a.com/deb stable main</code>
				<code>deb https://repo-b.com/deb stable main</code>
			`,
			wantDebs: []string{
				"deb https://repo-a.com/deb stable main",
				"deb https://repo-b.com/deb stable main",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tc.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			debs := FindDebLines(doc, false, nil)
			sort.Strings(debs)
			sort.Strings(tc.wantDebs)

			if !reflect.DeepEqual(debs, tc.wantDebs) {
				t.Errorf("Deb lines mismatch.\nGot:  %v\nWant: %v", debs, tc.wantDebs)
			}
		})
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.Contains(a[i], b[i]) && !strings.Contains(b[i], a[i]) {
			return false
		}
	}
	return true
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
