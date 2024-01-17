import configService from '@/services/config'

interface DashboardConfig {
  metrics_url?: string
  event_url?: string
  flow_url?: string
}

export default {
  state: {
      metrics_url: '',
      event_url: '',
      flow_url: ''
  } as DashboardConfig,
  reducers: {
      update(prevState, payload) {
          return {
            ...prevState,
            ...payload
          }
      }
  },
  effects: (dispatch) => ({
      async fetchDashboardConfig() {
          const data = await configService.getDashboardConfig()
          dispatch.dashboard.update(data)
      },
      async updateDashboardConfig(data, rootState) {
          const newData =  {
            ...rootState.dashboard,
            ...data
          }
          await configService.setDashboardConfig(newData)
          dispatch.dashboard.update(newData)
      }
  })
}
