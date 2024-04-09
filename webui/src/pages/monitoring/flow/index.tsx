import PageHeader from '@/components/PageHeader';
import FlowGraph from './components/FlowGraph';
import FlowTable from './components/FlowTable'
import { useEffect, useMemo, useState } from 'react';
import { Card, Select, Radio, Switch, Box, Message, Button, Icon, Loading, DatePicker2 } from '@alifd/next';
import styles from './index.module.css';
import flowService, { FlowData } from '@/services/flow'
import { getErrorMessage } from '@/utils';
import { Dayjs } from 'dayjs';
import * as dayjs from 'dayjs';
import { definePageConfig } from "ice";

const getNamespaces = (data: any) => {
  return data.nodes.map((node: any) => {
    return node.namespace || 'default'
  }).filter((v, i, a) => a.indexOf(v) === i).sort()
}

const filterFlowData = (data: FlowData, namespaces: string[], nodes: string[], showSeparate: boolean) => {
  let filteredNode = data.nodes.filter((node: any) => {
    return (namespaces?.length ?? 0) === 0 || node.type != 'pod' || namespaces.includes(node.namespace)
  })
    .filter((node: any) => {
      return !nodes.length || node.type != 'node' || nodes.includes(node.nodeName)
    })

  const nodeSet = new Set();
  filteredNode.forEach(i => {
    nodeSet.add(i.id);
  });

  const filteredEdge = data.edges.filter((edge: any) => {
    return nodeSet.has(edge.src) && nodeSet.has(edge.dst)
  });

  if (!showSeparate) {
    const s = new Set();
    filteredEdge.forEach(e => {
      s.add(e.src)
      s.add(e.dst)
    })

    filteredNode = filteredNode.filter(n => {
      return s.has(n.id)
    })
  }

  filteredNode.sort((a, b) => {
    return a.id.localeCompare(b.id)
  })

  filteredEdge.sort((a, b) => {
    return a.id.localeCompare(b.id)
  })

  return {
    nodes: filteredNode,
    edges: filteredEdge,
  }
}

export default function FlowDashboard() {
  const [data, setData] = useState({ nodes: [], edges: [] });
  const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [time, setTime] = useState<Dayjs[] | null>(null);

  const getFlowData = () => {
    const [from, to] = time || [dayjs().subtract(15, 'minute'), dayjs()];
    setLoading(true);
    flowService.getFlowData(from?.unix() || dayjs().subtract(15, 'minute').unix(), to?.unix() || dayjs().unix()).then((res) => {
      setData(res);
      setLoading(false);
    }).catch(err => {
      Message.error(`Error fetching data: ${getErrorMessage(err)}`)
    });
  };
  useEffect(() => getFlowData(), [time])

  const [viewMode, setViewMode] = useState('graph');
  const onViewModeChange = (value: string) => {
    setViewMode(value);
  }

  const namespaces = useMemo(() => getNamespaces(data), [data]);
  const [showSeparateEndpoints, setShowSeparateEndpoints] = useState(false);

  const onNamespacesChange = (value: string[]) => {
    setSelectedNamespaces(value);
  }

  const onShowExternalChange = (checked: boolean) => {
    setShowSeparateEndpoints(checked);
  }

  const filtered = useMemo(
    () => filterFlowData(data, selectedNamespaces, [], showSeparateEndpoints),
    [data, selectedNamespaces, showSeparateEndpoints]
  )

  return (
    <div>
      <PageHeader
        title='Network Graph'
        breadcrumbs={[{ name: 'Console' }, { name: 'Monitoring' }]}
      />
      <Card contentHeight="auto">
        <Card.Content style={{ paddingLeft: 0 }}>
          <Box direction="row" className={styles.contentBox}>
            <span className={styles.optionLabel}>Time Range</span>
            <DatePicker2.RangePicker placeholder={['Start Time', 'End Time']} showTime onChange={v => setTime(v)} />
          </Box>
          <Box className={styles.contentBox} direction='row'>
            <span className={styles.optionLabel}>Namespaces</span>
            <Select
              name="namespace"
              dataSource={namespaces}
              mode='multiple'
              onChange={onNamespacesChange}
              tagInline
              hasSelectAll
              showSearch
              hasClear
              useVirtual
              style={{ width: '100%', maxWidth: 300 }}
            />
          </Box>
          <Box direction='row'>
            <span className={styles.optionLabel}>Show Endpoints Without Link</span>
            <Switch id='showExternal' style={{ marginRight: '10px' }} onChange={onShowExternalChange} />
            <span className={styles.optionLabel}>ViewMode</span>
            <Radio.Group shape='button' defaultValue='graph' style={{ marginRight: '10px' }} onChange={onViewModeChange}>
              <Radio id='graph' value='graph'>Graph</Radio>
              <Radio id='table' value='table'>Table</Radio>
            </Radio.Group>
            <Button
              type="secondary"
              size="medium"
              style={{ marginLeft: 'auto', padding: "0 13px" }}
              onClick={() => getFlowData()}
            >
              <Icon type="refresh" />
              <span>Refresh</span>
            </Button>
          </Box>
        </Card.Content>
        <Card.Divider />
        <Card.Content style={{ padding: 0 }}>
          <Loading visible={loading} inline={false}>
            {viewMode === 'graph' && <FlowGraph data={filtered} />}
            {viewMode === 'table' && <FlowTable data={filtered} />}
          </Loading>
        </Card.Content>
      </Card>
    </div>
  );
}

export const pageConfig = definePageConfig(() => {
  return {
    title: 'Network Graph',
  };
});

