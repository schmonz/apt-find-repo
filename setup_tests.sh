#!/bin/sh
set -e

mkdir -p testdata/webpages

echo "Fetching test HTML files..."

# Already working
curl -s https://github.com/mwt/zoom-apt-repo > testdata/webpages/zoom-unofficial.html
curl -s https://github.com/JonasGroeger/jetbrains-ppa > testdata/webpages/jetbrains-unofficial.html
curl -s https://tailscale.com/download/linux/ubuntu-2204 > testdata/webpages/tailscale-official.html

# Development tools
curl -s https://docs.docker.com/engine/install/ubuntu/ > testdata/webpages/docker-official.html
curl -s https://code.visualstudio.com/docs/setup/linux > testdata/webpages/vscode-official.html
curl -s https://github.com/nodesource/distributions > testdata/webpages/nodejs-unofficial.html
curl -s https://github.com/cli/cli/blob/trunk/docs/install_linux.md > testdata/webpages/github-cli-official.html
curl -s https://yarnpkg.com/getting-started/install > testdata/webpages/yarn-official.html

# Databases
curl -s https://www.mongodb.com/docs/manual/tutorial/install-mongodb-on-ubuntu/ > testdata/webpages/mongodb-official.html
curl -s https://www.postgresql.org/download/linux/ubuntu/ > testdata/webpages/postgresql-official.html
curl -s https://redis.io/docs/latest/operate/oss_and_stack/install/install-redis/install-redis-on-linux/ > testdata/webpages/redis-official.html

# Infrastructure & DevOps
curl -s https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/ > testdata/webpages/kubernetes-official.html
curl -s https://docs.gitlab.com/runner/install/linux-repository.html > testdata/webpages/gitlab-runner-official.html
curl -s https://developer.hashicorp.com/terraform/install > testdata/webpages/terraform-official.html
curl -s https://nginx.org/en/linux_packages.html > testdata/webpages/nginx-official.html

# Monitoring & Observability
curl -s https://grafana.com/docs/grafana/latest/setup-grafana/installation/debian/ > testdata/webpages/grafana-official.html
curl -s https://www.elastic.co/guide/en/elasticsearch/reference/current/deb.html > testdata/webpages/elasticsearch-official.html
curl -s https://docs.influxdata.com/influxdb/latest/install/ > testdata/webpages/influxdb-official.html

# Browsers & Desktop Apps
curl -s https://www.google.com/chrome/ > testdata/webpages/chrome-official.html
curl -s https://brave.com/linux/ > testdata/webpages/brave-official.html
curl -s https://www.spotify.com/download/linux/ > testdata/webpages/spotify-official.html
curl -s https://slack.com/downloads/linux > testdata/webpages/slack-official.html
curl -s https://signal.org/download/linux/ > testdata/webpages/signal-official.html
curl -s https://www.sublimetext.com/docs/linux_repositories.html > testdata/webpages/sublime-official.html

# Other tools
curl -s https://apt.syncthing.net/ > testdata/webpages/syncthing-official.html
curl -s https://caddyserver.com/docs/install > testdata/webpages/caddy-official.html
curl -s https://support.1password.com/install-linux/ > testdata/webpages/1password-official.html

echo "Test data saved to testdata/webpages/"
echo "Total files: $(ls -1 testdata/webpages/ | wc -l)"
echo "Run: go test -v"
