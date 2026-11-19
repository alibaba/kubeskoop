# SNIC RDMA monitoring

Enable the existing `rdma` probe. SNIC `troce_*` devices use `type="troce"`.
SNIC metrics use the `kubeskoop_rdma_troce_*` prefix and retain native driver
counter names, including `out_of_seq`, `req_cqe_err`, and `resp_cqe_err`.
No SNIC values are mapped to mlx5 or vendor-neutral aliases. Existing mlx5 and
eRDMA metric definitions remain unchanged. No dashboard changes are included.

RDMA port labels are `device`, `type`, `port`. Netdev counters include
`device`, `type`, `bond`, `netdev`. PFC also includes `direction` and
`priority`. The node label is `k8s_node`. Device-to-bond-to-slave association
is discovered from sysfs; this topology association is not a metric alias.

Hardware statistics use netlink first, falling back per counter to sysfs
`hw_counters`; standard port counters are collected independently. Duplicate
series are suppressed. Missing or unsupported fields are omitted, never zero-filled.
Only registered fields are exported; `lifespan` and per-queue ethtool fields
are excluded. Unpopulated GID entries returning EINVAL are skipped.

`port_rcv_data` and `port_xmit_data` retain 4-byte-word units, verified with
fixed-size bidirectional RDMA writes. Query their rates
times four for bytes/second. They include protocol overhead.
Software `rx_bytes`/`tx_bytes` and MAC `mac_rx_bytes`/`mac_tx_bytes` remain
separate driver fields. Netdev statistics include non-RDMA traffic.
PFC duration retains raw driver units; do not infer seconds or pause percentage.
No CNP/ECN statistics are inferred from mlx5 fields.

The agent needs host network access for ethtool and read access to
`/sys/class/infiniband` and `/sys/class/net`, including symlink targets.
No external ethtool executable is needed. Verify visibility in the actual
agent before changing mounts. Ensure scrape/remote-write allowlists include
`kubeskoop_rdma_troce_.*`. Use rate/increase to account for counter resets.
