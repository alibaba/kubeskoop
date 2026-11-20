# SNIC RDMA monitoring

Enable the existing `rdma` probe. SNIC `troce_*` devices use `type="troce"`.
SNIC metrics use the `kubeskoop_rdma_troce_*` prefix and retain native driver
counter names, including `out_of_seq`, `req_cqe_err`, and `resp_cqe_err`.
No SNIC values are mapped to mlx5 or vendor-neutral aliases. Existing mlx5 and
eRDMA metric definitions remain unchanged.

The existing nodes dashboard at `deploy/resource/kubeskoop-exporter-nodes-dashboard.json`
includes troce queries in the RDMA receive/transmit byte-rate, packet-rate,
transmit-discard, and receive-buffer-drop panels. Import or update this dashboard
in Grafana to use these queries; upgrading the exporter alone does not update
an already imported dashboard. Existing mlx5/eRDMA queries are preserved.
The drop panels also include the existing eRDMA disabled-drop counters;
these are cumulative counts, not packet rates.

The 30 native RDMA counters use labels `device`, `type`, `port`, plus the
standard `k8s_node` label. No netdev or ethtool statistics are collected,
and no new dependency is required.

Collection uses the existing shared RDMA path: hardware statistics come from
RDMA netlink, and standard port statistics come from
`/sys/class/infiniband/<device>/ports/<port>/counters`. No driver-specific
fallback or filtering is added. As with mlx5/eRDMA, a failed netlink statistics
request skips port statistics for that device during that collection.

`port_rcv_data` and `port_xmit_data` retain 4-byte-word units, verified with
fixed-size bidirectional RDMA writes. Query their rates
times four for bytes/second. They include protocol overhead.
No CNP/ECN statistics are inferred from mlx5 fields.

The agent needs read access to `/sys/class/infiniband`, including symlink
targets. Verify visibility in the actual
agent before changing mounts. Ensure scrape/remote-write allowlists include
`kubeskoop_rdma_troce_.*`. Use rate/increase to account for counter resets.
