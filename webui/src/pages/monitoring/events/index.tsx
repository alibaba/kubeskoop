import PageHeader from '@/components/PageHeader';
import { Dayjs } from 'dayjs'
import { useEffect, useState, useMemo } from 'react';
import { Card, Box, DatePicker2, Button, Select, Icon, Message, Loading } from '@alifd/next';
import { EventData } from '@/services/event';
import eventService from '@/services/event';
import k8sService from '@/services/k8s';
import { NodeInfo, PodInfo } from '@/services/k8s';
import { getErrorMessage } from '@/utils';
import EventList from './components/EventList';
import styles from './index.module.css'
import { useRequest } from '@ice/plugin-request/hooks';

export default function Events() {
  const [eventData, setEventData] = useState<EventData[]>([]);

  const [nodes, setNodes] = useState<NodeInfo[]>([]);
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [pods, setPods] = useState<PodInfo[]>([]);
  const [timeRange, setTimeRange] = useState<Dayjs[]>([]);
  const [filteredTypes, setFilteredTypes] = useState<string[]>([]);
  const [filteredNodes, setFilteredNodes] = useState<string[]>([]);
  const [filteredNamespaces, setFilteredNamespaces] = useState<string[]>([]);
  const [filteredPods, setFilteredPods] = useState<string[]>([]);

  const [isLive, setIsLive] = useState(false);
  const [isLoading, setIsLoading] = useState(true);


  const [controller, setController] = useState<AbortController | null>(null);
  const refreshEventList = (setLoading: boolean) => {
    if (setLoading) setIsLoading(true);
    if (controller) controller.abort()
    const newController = new AbortController();
    const { signal } = newController;
    setController(newController);

    const start = timeRange[0], end = timeRange[1];
    eventService.getRangeEvent(start?.unix(), end?.unix(), filteredTypes, filteredNodes, filteredNamespaces, filteredPods, 100, signal).then((res) => {
      setEventData(res || [])
      if (setLoading) setIsLoading(false);
    }).catch((res) => {
      Message.error(`Error fetching events: ${getErrorMessage(res)}`)
    });
  };

  const refreshResourceList = () => {
    k8sService.listNodes().then((res) => {
      setNodes(res)
    }).catch((res) => {
      Message.error(`Error fetching node info: ${getErrorMessage(res)}`)
    }
    );
    k8sService.listNamespace().then((res) => {
      setNamespaces(res)
    }).catch((res) => {
      Message.error(`Error fetching namespace info: ${getErrorMessage(res)}`)
    }
    );
    k8sService.listPods().then((res) => {
      setPods(res)
    }).catch((res) => {
      Message.error(`Error fetching pod info: ${getErrorMessage(res)}`)
    }
    );
  }

  useEffect(() => {
    refreshEventList(true);
  }, [timeRange, filteredTypes, filteredNodes, filteredNamespaces, filteredPods])

  useEffect(() => {
    refreshResourceList();
  }, [])

  useEffect(() => {
    if (isLive) {
      const t = setInterval(() => {
        refreshEventList(false);
      }, 2000);
      return () => {
        clearInterval(t);
      }
    }
    return () => { }
  }, [isLive])

  const podNames = useMemo(() => {
    return filteredNamespaces?.length ?
      pods.filter((i) => {
        return filteredNamespaces.includes(i.namespace)
      }).map(i => i.name) : pods.map(i => i.name);
  }, [pods, filteredNamespaces])

  const nodeNames = useMemo(() => nodes.map(i => i.name), [nodes])
  const eventTypes = useMemo(() => {
    return eventData.map((i) => i.type).filter((i, index) => eventData.findIndex((j) => j.type === i) === index)
  }, [eventData])

  return (
    <div>
      <PageHeader
        title='Events'
        breadcrumbs={[{ name: 'Console' }, { name: 'Monitoring' }, { name: 'Events' }]}
      />
      <Card contentHeight="auto">
        <Card.Content>
          <Box direction="row" className={styles.box}>
            <span className={styles.title}>Time Range</span>
            <DatePicker2.RangePicker showTime disabled={isLive} onChange={v => setTimeRange(v)} />
          </Box>
          <Box direction="row" className={styles.box}>
            <span className={styles.title}>Event Type</span>
            <Select className={styles.select} mode="tag" dataSource={eventTypes} onChange={v => setFilteredTypes(v)} disabled={isLive} />
            <span className={styles.title} style={{width: 50, minWidth: 50}}>Nodes</span>
            <Select className={styles.select} mode="tag" dataSource={nodeNames} onChange={v => setFilteredNodes(v)} disabled={isLive} />
            </Box>
            <Box direction="row" className={styles.box}>
            <span className={styles.title}>Namespaces</span>
            <Select className={styles.select} mode="tag" dataSource={namespaces} onChange={v => setFilteredNamespaces(v)} disabled={isLive} />
            <span className={styles.title} style={{width: 50, minWidth: 50}}>Pods</span>
            <Select className={styles.select} mode="tag" dataSource={podNames} onChange={v => setFilteredPods(v)} disabled={isLive} />
            <div style={{ marginLeft: "auto", paddingLeft: 10, display: 'flex' }}>
              <Button
                disabled={isLive}
                className={styles.btn}
                type="secondary"
                size="medium"
                style={{ marginLeft: 'auto', padding: "0 13px" }}
                onClick={() => refreshEventList(true)}
              >
                <Icon type="refresh" />
                Refresh
              </Button>
              {
                isLive ?
                  <Button className={styles.btn} type="secondary" size="medium" style={{ marginLeft: 'auto', padding: "0 13px" }} onClick={() => setIsLive(false)}><Icon type="loading" />Pause</Button> :
                  <Button className={styles.btn} type="secondary" size="medium" onClick={() => setIsLive(true)}>Live</Button>
              }
            </div>
          </Box>
        </Card.Content>
        <Card.Header title="Event List" />
        <Card.Divider />
        <Card.Content>
          <Loading visible={isLoading} style={{ display: 'block' }}>
            <EventList data={eventData} />
          </Loading>
        </Card.Content>
      </Card>
    </div>
  );
}
