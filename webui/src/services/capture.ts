import { request } from 'ice'
import {NodeInfo, PodInfo} from "@/services/k8s";

export interface CaptureTask {
    pod: PodInfo
    node: NodeInfo
    capture_host_ns: boolean
    capture_duration_seconds: number
}

export interface CaptureResult {
    task_id: number,
    task_config: CaptureTask,
    status: string,
    result: string,
    message: string
}

export default {
    async createCapture(task: CaptureTask): Promise<string> {
        return await request({
            url: '/controller/capture',
            method: 'POST',
            data: task,
        });
    },
    async listCaptures(): Promise<string> {
        return await request({
            url: '/controller/captures',
            method: 'GET',
        });
    }
};
