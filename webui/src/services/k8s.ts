import { request } from 'ice'

export interface PodInfo {
    name: string
    namespace: string
    nodename: string
    labels: object
}

export interface NodeInfo {
    name: string
    labels: object
}

export default {
    async listPods(): Promise<PodInfo[]> {
        return await request({
            url: '/controller/pods',
            method: 'GET',
        });
    },
    async listNodes(): Promise<NodeInfo[]> {
        return await request({
            url: '/controller/nodes',
            method: 'GET',
        });
    },
    async listNamespace(): Promise<string[]> {
        return await request({
            url: '/controller/namespaces',
            method: 'GET',
        });
    },
};
