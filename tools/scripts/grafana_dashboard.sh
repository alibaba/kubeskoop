#!/usr/bin/env bash
set -e
set -x

trap "exit 0" 15

GRAFANA_HOST=${GRAFANA_HOST:-"127.0.0.1:3000"}
GRAFANA_PASSWORD=${GRAFANA_PASSWORD:-kubeskoop}

register_dashboard() {
    local dashboard='{}'
    local datasource_id=0
    #dashboard=$(curl -sSL https://raw.githubusercontent.com/alibaba/kubeskoop/main/deploy/resource/kubeskoop-exporter-dashboard.json)
    dashboard=$(cat /etc/kubeskoop-exporter-dashboard.json)
    datasource_id=$(curl "http://admin:$GRAFANA_PASSWORD@$GRAFANA_HOST/api/datasources/name/prometheus" | jq .uid)
    tmp_dashboard_file=$(mktemp)
    cat <<EOF > "${tmp_dashboard_file}"
{
    "dashboard": $dashboard,
    "overwrite": true,
    "inputs": [
        {
            "name": "DS_PROMETHEUS",
            "type": "datasource",
            "pluginId": "prometheus",
            "value": $datasource_id
        }
    ],
    "folderUid": ""
}
EOF
    curl "http://admin:$GRAFANA_PASSWORD@$GRAFANA_HOST/api/dashboards/import" \
      -H 'content-type: application/json' \
      --data  @"${tmp_dashboard_file}"
}

grafana_ready() {
    local n=0
    while [[ $n -lt 10 ]]; do
        # shellcheck disable=SC2068
        if curl "http://admin:$GRAFANA_PASSWORD@$GRAFANA_HOST/api/datasources/name/prometheus" &> /dev/null; then
            return 0
        else
            n=$((n + 1))
            sleep 5
        fi
    done
    echo "timeout wait grafana ready"
    exit 1
}

grafana_ready
register_dashboard
sleep infinity
