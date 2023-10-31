import { request } from 'ice'

export interface PodInfo {
    name: string
    namespace: string
}

export interface NodeInfo {
    name: string
}

export default {
    async list_pods(): Promise<PodInfo[]> {
        return await request({
            url: '/controller/pods',
            method: 'GET',
        });
    },
    async list_nodes(): Promise<NodeInfo[]> {
        return await request({
            url: '/controller/nodes',
            method: 'GET',
        });
    },
};
