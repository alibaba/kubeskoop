import { request } from 'ice'

export interface NodeInfo {
    name: string
    namespace: string
    nodename: string
    type: string
}

export interface Latency {
    source: NodeInfo
    destination: NodeInfo
    latency_avg: number
    latency_max: number
    latency_min: number
}

export interface PingMeshLatency {
    nodes: NodeInfo[]
    latencies: Latency[]
}

export interface PingMeshArgs {
    ping_mesh_list: NodeInfo[]
}

export default {
    async pingMeshLatency(args: PingMeshArgs): Promise<PingMeshLatency> {
        return await request({
            url: '/controller/pingmesh',
            method: 'POST',
            data: args,
        });
    },
};
