import { request } from 'ice'

export default {
    async getDashboardConfig(): Promise<any> {
        return await request({
            url: `/config/dashboard`,
        });
    },
    async setDashboardConfig(value: any): Promise<any> {
        return await request({
            url: `http://127.0.0.1:8080/config/dashboard`,
            method: 'PUT',
            data: value,
        });
    }
}
