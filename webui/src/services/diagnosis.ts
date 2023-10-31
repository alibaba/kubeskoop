import { request } from 'ice'

export interface DiagnosisDestination {
    address: string
    port: number
}

export interface DiagnosisTask {
    source: string
    destination: DiagnosisDestination
    protocol: string
}

export interface DiagnosisResult {
    task_id: string
    task_config: DiagnosisTask
    start_time: string
    finish_time: string
    status: string
    result: string
    message: string
}

export default {
    async listDiagnosis(): Promise<DiagnosisResult[]> {
        return await request({
            url: '/diagnosis',
        });
    },
    async getDiagnosisById(id): Promise<DiagnosisResult> {
        return await request({
            url: `/diagnosis/${id}`,
        });
    },
    async createDiagnosis(task: DiagnosisTask): Promise<string> {
        return await request({
            url: '/diagnosis',
            method: 'POST',
            data: task,
        });
    }
};
