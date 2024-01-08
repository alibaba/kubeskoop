import { request } from 'ice'

export interface EventLabel {
  name: string
  value: string
}

export interface EventData {
  node: string
  time: number
  timestamp: number
  type: string
  labels: EventLabel[]
  msg: string
};

export default {
  async getRangeEvent(start?: number, end?: number, types: string[], nodes?: string[], namespaces?: string[], pods?: string[], limit?: number): Promise<EventData[]> {
    return request.get('/controller/events', {
      params: {
        start,
        end,
        types: types?.join(','),
        nodes: nodes?.join(','),
        namespaces: namespaces?.join(','),
        pods: pods?.join(','),
        limit,
      },
    });
  }
};
