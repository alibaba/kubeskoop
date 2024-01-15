import { Dialog, Select, Box, Input, Message } from "@alifd/next"
import { useState } from "react"
import styles from "./index.module.css"
import _ from "lodash"

export interface SelectableItem {
  name: string
  args: any
}

export interface Selection {
  name: string
  args: any
}

interface AddDialogProps {
  visible: boolean
  items: SelectableItem[]
  autoComplete?: boolean
  type: string
  onOk: (type: string, selection: Selection) => void
  onCancel: () => void
}

interface LokiArgsProps {
  onChange: (args: any) => void
}

const LokiArgs: React.FC<LokiArgsProps> = (props: LokiArgsProps): JSX.Element => {
  const onChange = (v: string) => {
    const args = {
      addr: v
    };
    props.onChange(args);
  }

  return (
    <div>
      <Box direction="row" className={styles.box}>
        <span>地址</span>
        <Input onChange={onChange} placeholder="loki-service" style={{ width: 300 }} />
      </Box>
    </div>
  )
}


const AddDialog: React.FC<AddDialogProps> = (props: AddDialogProps): JSX.Element => {
  const [selection, setSelection] = useState<string>('');
  const [args, setArgs] = useState<any>(undefined);
  const verifyInput = (): boolean => {
    if (selection === '') {
      Message.error('请输入名称');
      return false;
    }
    if (props.type === 'event_sink' && selection === 'loki' && _.isEmpty(args?.addr)) {
      Message.error('请输入地址');
      return false;
    }
    return true;
  }

  const onOk = () => {
    if (!verifyInput()) {
      return;
    }
    props.onOk(props.type, {
      name: selection,
      args: args
    })
  }

  return <Dialog
    v2
    title="添加"
    visible={props.visible}
    onOk={onOk}
    onCancel={() => props.onCancel()}
    onClose={() => props.onCancel()}
  >
    <Box direction="row" className={styles.box}>
      <span>名称</span>
      {
        props.autoComplete ?
          <Select.AutoComplete
            style={{ width: 300 }}
            dataSource={props.items.map(i => i.name)}
            value={selection}
            onChange={v => setSelection(v)}
          />
          :
          <Select
            style={{ width: 300 }}
            showSearch
            dataSource={props.items.map(i => i.name)}
            value={selection}
            onChange={v => setSelection(v)}
          />
      }
    </Box>
    {
      props.type === 'event_sink' && selection === 'loki' ?
      <LokiArgs onChange={setArgs} /> : null
    }
  </Dialog>
}

export default AddDialog;
