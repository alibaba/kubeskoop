import { request } from 'ice'

export interface CaptureTask {
    task_type: string
    name: string
    namespace: string
}

export interface CaptureResult {
    task_id: number,
    spec: CaptureTask,
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
