import { Card, Table, Button, List, Affix, Message } from "@alifd/next";
import configService, { ExporterConfig } from "@/services/config"
import PageHeader from "@/components/PageHeader";
import styles from "./index.module.css"
import { useEffect, useState } from "react";
import AddDialog, { SelectableItem, Selection } from "./components/AddDialog";
import probeTypes from "./types.json"
import { getErrorMessage } from "@/utils"
import _ from "lodash"

const METRIC_PROBE = "metric_probe"
const EVENT_PROBE = "event_probe"
const EVENT_SINK = "event_sink"

export default function Config() {
  const [exporterConfig, setExporterConfig] = useState<ExporterConfig | null>(null);
  useEffect(() => {
    configService.getExporterConfig().then(setExporterConfig);
  }, []);

  const [dialogVisible, setDialogVisible] = useState(false);
  const [configChanged, setConfigChanged] = useState(false);

  const extractMetricProbes = (cfg: ExporterConfig | null) => cfg?.metrics?.probes || [];
  const extractEventProbes = (cfg: ExporterConfig | null) => cfg?.event?.probes || [];
  const extractEventSinks = (cfg: ExporterConfig | null) => cfg?.event?.sinks || [];

  const generateActions = (type: string, index: number): JSX.Element => {
    return (
      <div>
        <Button warning
          text
          style={{ color: 'red' }}
          onClick={() => deleteItem(type, index)}
        >
          删除
        </Button>
      </div>
    )
  }

  const deleteItem = (type: string, index: number) => {
    // we do not need a deep copy here
    const newConfig = { ...exporterConfig } as ExporterConfig
    switch (type) {
      case METRIC_PROBE:
        newConfig.metrics?.probes?.splice(index, 1);
        break;
      case EVENT_PROBE:
        newConfig.event?.probes?.splice(index, 1);
        break;
      case EVENT_SINK:
        newConfig.event?.sinks?.splice(index, 1);
        break;
      default:
        console.log(`unknown item type ${type}!`);
        break;
    }
    setExporterConfig(newConfig);
    setConfigChanged(true);
  }

  const [selectableItems, setSelectableItems] = useState<SelectableItem[]>([]);
  const [dialogType, setDialogType] = useState<string>('');

  const showAddItemDialog = (type: string) => {
    switch (type) {
      case METRIC_PROBE:
        setSelectableItems(probeTypes.metric_probe);
        break;
      case EVENT_PROBE:
        setSelectableItems(probeTypes.event_probe);
        break;
      case EVENT_SINK:
        setSelectableItems(probeTypes.event_sink);
        break;
    }
    setDialogType(type);
    setDialogVisible(true);
  }

  const onAddSelection = (type: string, selection: Selection) => {
    console.log(selection)
    const newConfig = { ...exporterConfig } as ExporterConfig
    switch (type) {
      case METRIC_PROBE:
        if (newConfig.metrics?.probes == null) {
          newConfig.metrics.probes = [];
        }
        if (newConfig.metrics?.probes.find(p => p.name === selection.name)) {
          Message.error(`指标探针 ${selection} 已存在!`)
          return
        }
        newConfig.metrics?.probes?.push({ ...selection });
        break;
      case EVENT_PROBE:
        if (newConfig.event?.probes == null) {
          newConfig.event.probes = [];
        }
        if (newConfig.event?.probes.find(p => p.name === selection.name)) {
          Message.error(`事件探针 ${selection} 已存在!`)
          return
        }
        newConfig.event?.probes?.push({ ...selection });
        break;
      case EVENT_SINK:
        if (newConfig.event?.sinks == null) {
          newConfig.event.sinks = [];
        }
        if (newConfig.event?.sinks.find(p => p.name === selection.name)) {
          Message.error(`事件投递 ${selection.name} 已存在!`)
          return
        }
        newConfig.event?.sinks?.push({ ...selection });
        break;
      default:
        console.log(`unknown item type ${type}!`);
        break;
    }
    setExporterConfig(newConfig);
    setDialogVisible(false);
    setConfigChanged(true);
  }

  const saveConfig = () => {
    console.log(exporterConfig)
    configService.updateExporterConfig(exporterConfig!).then(() => {
      Message.success('配置保存成功')
      setConfigChanged(false);
    }).catch(e => {
      Message.error(`配置保存失败: ${getErrorMessage(e)}`)
    })
  }

  return (
    <div>
      <PageHeader
        title='节点配置'
        breadcrumbs={[{ name: 'Console' }, { name: '配置' }, { name: '节点配置' }]}
      />
      <Card title="指标探针" contentHeight="auto">
        <div style={{ display: 'flex' }}>
          <Button type='primary' style={{ marginBottom: 10 }} onClick={() => showAddItemDialog(METRIC_PROBE)}>添加</Button>
        </div>
        <Table.StickyLock dataSource={extractMetricProbes(exporterConfig)}>
          <Table.Column title="名称" dataIndex="name" />
          <Table.Column title="参数" dataIndex="args" cell={v => _.isEmpty(v) ? '-' : JSON.stringify(v)} />
          <Table.Column title="操作" cell={(_, i) => generateActions('metric_probe', i)} />
        </Table.StickyLock>
      </Card>
      <Card title="事件探针" contentHeight="auto">
        <div style={{ display: 'flex' }}>
          <Button type='primary' style={{ marginBottom: 10 }} onClick={() => showAddItemDialog(EVENT_PROBE)}>添加</Button>
        </div>
        <Table.StickyLock dataSource={extractEventProbes(exporterConfig)}>
          <Table.Column title="名称" dataIndex="name" />
          <Table.Column title="参数" dataIndex="args" cell={v => _.isEmpty(v) ? '-' : JSON.stringify(v)} />
          <Table.Column title="操作" cell={(_, i) => generateActions('event_probe', i)} />
        </Table.StickyLock>
      </Card>
      <Card title="事件投递" contentHeight="auto">
        <div style={{ display: 'flex' }}>
          <Button type='primary' style={{ marginBottom: 10 }} onClick={() => showAddItemDialog(EVENT_SINK)}>添加</Button>
        </div>
        <Table.StickyLock dataSource={extractEventSinks(exporterConfig)}>
          <Table.Column title="名称" dataIndex="name" />
          <Table.Column title="参数" dataIndex="args" cell={v => _.isEmpty(v) ? '-' : JSON.stringify(v)} />
          <Table.Column title="操作" cell={(_, i) => generateActions('event_sink', i)} />
        </Table.StickyLock>
      </Card>
      <Card contentHeight="auto">
        <Affix offsetBottom={20}>
          <div style={{ display: 'flex' }}>
            <Button disabled={!configChanged} onClick={saveConfig} type="primary" style={{ marginLeft: 'auto' }}>保存配置</Button>
          </div>
        </Affix>
      </Card>
      <AddDialog
        visible={dialogVisible}
        items={selectableItems}
        type={dialogType}
        autoComplete
        onOk={onAddSelection}
        onCancel={() => setDialogVisible(false)}
      />
    </div>
  );
}
