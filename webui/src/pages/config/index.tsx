import { Card, Table, Button, List, Affix, Message } from "@alifd/next";
import configService, { ExporterConfig } from "@/services/config"
import PageHeader from "@/components/PageHeader";
import styles from "./index.module.css"
import { useEffect, useState } from "react";
import AddDialog, { SelectableItem, Selection } from "./components/AddDialog";
import probeTypes from "./types.json"
import { getErrorMessage } from "@/utils"
import _ from "lodash"
import { definePageConfig } from "ice";

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
          Delete
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
        const enabledMetricProbes = extractMetricProbes(exporterConfig)?.map(p => p.name);
        setSelectableItems(probeTypes.metric_probe.filter(i => !enabledMetricProbes.includes(i.name)));
        break;
      case EVENT_PROBE:
        const enabledEventProbes = extractEventProbes(exporterConfig)?.map(p => p.name);
        setSelectableItems(probeTypes.event_probe.filter(i => !enabledEventProbes.includes(i.name)));
        break;
      case EVENT_SINK:
        const enabledEventSinks = extractEventSinks(exporterConfig)?.map(p => p.name);
        setSelectableItems(probeTypes.event_sink.filter(i => !enabledEventSinks.includes(i.name)));
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
          Message.error(`Metric probe \"${selection.name}\" has already existed!`)
          return
        }
        newConfig.metrics?.probes?.push({ ...selection });
        break;
      case EVENT_PROBE:
        if (newConfig.event?.probes == null) {
          newConfig.event.probes = [];
        }
        if (newConfig.event?.probes.find(p => p.name === selection.name)) {
          Message.error(`Event probe \"${selection.name}\" has already existed!`)
          return
        }
        newConfig.event?.probes?.push({ ...selection });
        break;
      case EVENT_SINK:
        if (newConfig.event?.sinks == null) {
          newConfig.event.sinks = [];
        }
        if (newConfig.event?.sinks.find(p => p.name === selection.name)) {
          Message.error(`Event sink \"${selection.name}\" has already existed!`)
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
      Message.success('Configuration saved.')
      setConfigChanged(false);
    }).catch(e => {
      Message.error(`Error when saving configuration: ${getErrorMessage(e)}`)
    })
  }

  return (
    <div>
      <PageHeader
        title='Node Configuration'
        breadcrumbs={[{ name: 'Console' }, { name: 'Configuration' }, { name: 'Node Configuration' }]}
      />
      <Card title="Metric Probes" contentHeight="auto">
        <div style={{ display: 'flex' }}>
          <Button type='primary' style={{ marginBottom: 10 }} onClick={() => showAddItemDialog(METRIC_PROBE)}>Add</Button>
        </div>
        <Table.StickyLock dataSource={extractMetricProbes(exporterConfig)}>
          <Table.Column title="Name" dataIndex="name" />
          <Table.Column title="Args" dataIndex="args" cell={v => _.isEmpty(v) ? '-' : JSON.stringify(v)} />
          <Table.Column title="Actions" cell={(_, i) => generateActions(METRIC_PROBE, i)} />
        </Table.StickyLock>
      </Card>
      <Card title="Event Probes" contentHeight="auto">
        <div style={{ display: 'flex' }}>
          <Button type='primary' style={{ marginBottom: 10 }} onClick={() => showAddItemDialog(EVENT_PROBE)}>Add</Button>
        </div>
        <Table.StickyLock dataSource={extractEventProbes(exporterConfig)}>
          <Table.Column title="Name" dataIndex="name" />
          <Table.Column title="Args" dataIndex="args" cell={v => _.isEmpty(v) ? '-' : JSON.stringify(v)} />
          <Table.Column title="Actions" cell={(_, i) => generateActions(EVENT_PROBE, i)} />
        </Table.StickyLock>
      </Card>
      <Card title="Event Sinks" contentHeight="auto">
        <div style={{ display: 'flex' }}>
          <Button type='primary' style={{ marginBottom: 10 }} onClick={() => showAddItemDialog(EVENT_SINK)}>Add</Button>
        </div>
        <Table.StickyLock dataSource={extractEventSinks(exporterConfig)}>
          <Table.Column title="Name" dataIndex="name" />
          <Table.Column title="Args" dataIndex="args" cell={v => _.isEmpty(v) ? '-' : JSON.stringify(v)} />
          <Table.Column title="Actions" cell={(_, i) => generateActions(EVENT_SINK, i)} />
        </Table.StickyLock>
      </Card>
      <Card contentHeight="auto">
        <Affix offsetBottom={20}>
          <div style={{ display: 'flex' }}>
            <Button disabled={!configChanged} onClick={saveConfig} type="primary" style={{ marginLeft: 'auto' }}>Save Configuration</Button>
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

export const pageConfig = definePageConfig(() => {
  return {
    title: 'Node Configuration',
  };
});
