import { request } from 'ice'

export interface Node {
    id: string
    ip: string
    type: string
    name: string
    namespace: string
    node_name: string
};

export interface Edge {
    id: string
    src: string
    dst: string
};

export interface FlowData {
    nodes: Node[]
    edges: Edge[]
};

export default {
    async getFlowData(time: number) {
        return await request({
            url: '/controller/flow',
            method: 'GET',
            params: {
                time,
            }
        });
    },
}
