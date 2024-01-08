import PageHeader from '@/components/PageHeader';
import { Dayjs } from 'dayjs'
import { useEffect, useState, useMemo } from 'react';
import { Card, Box, DatePicker2, Button, Select, Icon, Message, Loading } from '@alifd/next';
import { EventData } from '@/services/event';
import eventService from '@/services/event';
import k8sService from '@/services/k8s';
import { getErrorMessage } from '@/utils';
import EventList from './components/EventList';
import styles from './index.module.css'

export default function Events() {
  const [eventData, setEventData] = useState<EventData[]>([]);

  const [nodes, setNodes] = useState<string[]>([]);
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [pods, setPods] = useState<string[]>([]);
  const [timeRange, setTimeRange] = useState<Dayjs[]>([]);
  const [filteredTypes, setFilteredTypes] = useState<string[]>([]);
  const [filteredNodes, setFilteredNodes] = useState<string[]>([]);
  const [filteredNamespaces, setFilteredNamespaces] = useState<string[]>([]);
  const [filteredPods, setFilteredPods] = useState<string[]>([]);

  const [isLive, setIsLive] = useState(false);
  const [isLoading, setIsLoading] = useState(true);

  const refreshEventList = (setLoading: boolean) => {
    if (setLoading) setIsLoading(true);
    const now = Math.floor(Date.now() / 1000)
    console.log(now)
    const start = timeRange[0], end = timeRange[1];
    eventService.getRangeEvent(start?.unix(), end?.unix(), filteredTypes, filteredNodes, filteredNamespaces, filteredPods).then((res) => {
      setEventData(res || [])
      if (setLoading) setIsLoading(false);
    }).catch((res) => {
      Message.error(`获取事件信息失败：${getErrorMessage(res)}`)
    });
  };

  const refreshResourceList = () => {
    k8sService.listNodes().then((res) => {
      setNodes(res.map((i) => i.name))
    }).catch((res) => {
      Message.error(`获取节点信息失败：${getErrorMessage(res)}`)
    }
    );
    k8sService.listNamespace().then((res) => {
      setNamespaces(res)
    }).catch((res) => {
      Message.error(`获取命名空间信息失败：${getErrorMessage(res)}`)
    }
    );
    k8sService.listPods().then((res) => {
      setPods(res.map((i) => i.name))
    }).catch((res) => {
      Message.error(`获取Pod信息失败：${getErrorMessage(res)}`)
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

  const namespacedPods = useMemo(() => {
    return filteredNamespaces?.length ?
      pods.filter((i) => {
        return filteredNamespaces.includes(i.namespace)
      }) : pods;
  }
    , [pods, filteredNamespaces])

  const eventTypes = useMemo(() => {
    return eventData.map((i) => i.type).filter((i, index) => eventData.findIndex((j) => j.type === i) === index)
  }, [eventData])

  return (
    <div>
      <PageHeader
        title='事件'
        breadcrumbs={[{ name: 'Console' }, { name: '监控' }, { name: '事件' }]}
      />
      <Card title="事件" contentHeight="auto">
        <Card.Content>
          <Box direction="row" className={styles.box}>
            <span className={styles.title}>时间范围</span>
            <DatePicker2.RangePicker showTime disabled={isLive} onChange={v => setTimeRange(v)} />
          </Box>
          <Box direction="row" className={styles.box}>
            <span className={styles.title}>事件类型</span>
            <Select className={styles.select} mode="tag" dataSource={eventTypes} onChange={v => setFilteredTypes(v)} disabled={isLive}/>
            <span className={styles.title}>节点</span>
            <Select className={styles.select} mode="tag" dataSource={nodes} onChange={v => setFilteredNodes(v)} disabled={isLive}/>
            <span className={styles.title}>命名空间</span>
            <Select className={styles.select} mode="tag" dataSource={namespaces} onChange={v => setFilteredNamespaces(v)} disabled={isLive}/>
            <span className={styles.title}>Pod名称</span>
            <Select className={styles.select} mode="tag" dataSource={namespacedPods} onChange={v => setFilteredPods(v)} disabled={isLive}/>
            <div style={{ marginLeft: "auto" }}>
              <Button
                disabled={isLive}
                className={styles.btn}
                type="secondary"
                size="medium"
                style={{ marginLeft: 'auto', padding: "0 13px" }}
                onClick={() => refreshEventList(true)}
              >
                <Icon type="refresh" />
                刷新
              </Button>
              {
                isLive ?
                  <Button className={styles.btn} type="secondary" size="medium" style={{ marginLeft: 'auto', padding: "0 13px" }} onClick={() => setIsLive(false)}><Icon type="loading" />暂停</Button> :
                  <Button className={styles.btn} type="secondary" size="medium" onClick={() => setIsLive(true)}>实时</Button>
              }
            </div>
          </Box>
        </Card.Content>
        <Card.Header title="事件列表" />
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
