import { request } from 'ice'

export interface ExporterConfig {
    debugMode: boolean
    port: number
    enableController: boolean
    metrics: MetricsConfig
    event: EventConfig
}

export interface MetricsConfig {
    probes: Probe[]
}

export interface Probe {
    name: string
    args?: object
}

export interface EventConfig {
    sinks: Sink[]
    probes: Probe2[]
}

export interface Sink {
    name: string
    args?: object
}

export interface Args2 {
    addr: string
}

export interface Probe2 {
    name: string
    args?: object
}

export interface Args3 { }

export default {
    async getDashboardConfig(): Promise<any> {
        return await request({
            url: `/config/dashboard`,
        });
    },
    async setDashboardConfig(value: any): Promise<any> {
        return await request({
            url: `/config/dashboard`,
            method: 'PUT',
            data: value,
        });
    },
    async getExporterConfig(): Promise<ExporterConfig> {
        return await request({
            url: `/controller/config`,
        });
    },
    async updateExporterConfig(value: ExporterConfig): Promise<any> {
        return await request({
            url: `/controller/config`,
            method: 'PUT',
            data: value,
        });
    },
}
