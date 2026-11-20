# SNIC RDMA monitoring

Enable the existing `rdma` probe. SNIC `troce_*` devices use `type="troce"`.
SNIC metrics use the `kubeskoop_rdma_troce_*` prefix and retain native driver
counter names, including `out_of_seq`, `req_cqe_err`, and `resp_cqe_err`.
No SNIC values are mapped to mlx5 or vendor-neutral aliases. Existing mlx5 and
eRDMA metric definitions remain unchanged. No dashboard changes are included.

The 30 native RDMA counters use labels `device`, `type`, `port`, plus the
standard `k8s_node` label. No netdev or ethtool statistics are collected,
and no new dependency is required.

Hardware statistics use netlink first, falling back per counter to sysfs
`hw_counters`; standard port counters are collected independently. Duplicate
series are suppressed. Missing or unsupported fields are omitted, never zero-filled.
Only registered fields are exported; `lifespan`
are excluded. Unpopulated GID entries returning EINVAL are skipped.

`port_rcv_data` and `port_xmit_data` retain 4-byte-word units, verified with
fixed-size bidirectional RDMA writes. Query their rates
times four for bytes/second. They include protocol overhead.
No CNP/ECN statistics are inferred from mlx5 fields.

The agent needs read access to `/sys/class/infiniband`, including symlink
targets. Verify visibility in the actual
agent before changing mounts. Ensure scrape/remote-write allowlists include
`kubeskoop_rdma_troce_.*`. Use rate/increase to account for counter resets.
