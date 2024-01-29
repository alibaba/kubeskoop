import PageHeader from '@/components/PageHeader';
import FlowGraph from './components/FlowGraph'
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
  }).filter((item, index, arr) => {
    return arr.indexOf(item) === index
  })
}

const filterFlowData = (data: FlowData, namespaces: string[], nodes: string[], showExternal: boolean) => {
  const filteredNode = data.nodes.filter((node: any) => {
    return !namespaces || node.type != 'pod' || namespaces.includes(node.namespace)
  })
    .filter((node: any) => {
      return !node.length || node.type == 'external' || nodes.includes(node.nodeName)
    })
    .filter((node: any) => {
      return showExternal || node.type !== 'external'
    });

  const nodeMap = {};
  filteredNode.forEach(i => {
    nodeMap[i.id] = true
  });

  const filteredEdge = data.edges.filter((edge: any) => {
    return nodeMap[edge.src] && nodeMap[edge.dst]
  });

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
  const [time, setTime] = useState<Dayjs | null>(null);

  const getFlowData = (initial: boolean) => {
    setLoading(true);
    flowService.getFlowData(time?.unix() || dayjs().unix()).then((res) => {
      if (initial) setSelectedNamespaces(getNamespaces(res));
      setData(res);
      setLoading(false);
    }).catch(err => {
      Message.error(`Error fetching data: ${getErrorMessage(err)}`)
    });
  };
  useEffect(() => getFlowData(true), []);
  useEffect(() => getFlowData(false), [time])

  const [viewMode, setViewMode] = useState('graph');
  const onViewModeChange = (value: string) => {
    setViewMode(value);
  }

  const namespaces = useMemo(() => getNamespaces(data), [data]);
  const [showExternal, setShowExternal] = useState(false);

  const onNamespacesChange = (value: string[]) => {
    console.log(value);
    setSelectedNamespaces(value);
  }

  const onShowExternalChange = (checked: boolean) => {
    setShowExternal(checked);
  }

  const filtered = useMemo(
    () => filterFlowData(data, selectedNamespaces, [], showExternal),
    [data, selectedNamespaces, showExternal]
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
            <span className={styles.optionLabel}>Time</span>
            <DatePicker2 showTime onChange={v => setTime(v)} />
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
            <span className={styles.optionLabel}>Show External Endpoints</span>
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
              onClick={() => getFlowData(false)}
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

