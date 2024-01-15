import { Form, Input, Select, Radio, Checkbox, TimePicker, Button, Message } from '@alifd/next';
import { useEffect, useState } from "react";
import styles from "./index.module.css"
import moment from 'moment';
import k8sService from "@/services/k8s";
import SelectorDialog from "./selectorDialog.tsx"

interface CaptureFormProps {
  onSubmit: (data: CaptureFormData) => void;
}

interface CaptureFormData {
  [key: string]: any;
}



const CaptureForm: React.FunctionComponent<CaptureFormProps> = (props: CaptureFormProps) => {
  const { onSubmit } = props;
  const [showSelectorDialog, setShowSelectorDialog] = useState(false)

  const [capturelist, setCapturelist] = useState([])
  const handleSubmit = (values: CaptureFormData, errors: any) => {
    if (errors) {
      return
    }
    if (capturelist.length == 0) {
      Message.error("Please select at least one target.")
      return
    }
    values["capture_list"] = capturelist
    values["duration"] = values["duration"].minutes() * 60 + values["duration"].seconds()
    onSubmit(values);
    console.log(values)
  };

  return (
    <Form inline labelAlign='left'>
      <Form.Item label="Targets" >
        <div className={styles.custom}>
          {capturelist.map((v, i) => {
            if (v.type == "Node") {
              return <Button className={styles.btn} key={i}>{v.type + ": " + v.name}</Button>;
            } else {
              return <Button className={styles.btn} key={i}>{v.type + ": " + v.namespace + "/" + v.name}</Button>;
            }
          })}
          <Button className={styles.btn} type="primary" onClick={() => { setShowSelectorDialog(!showSelectorDialog) }}>
            Add ＋{" "}
          </Button>
          <Button className={styles.btn} warning type="primary" onClick={() => { setCapturelist([]) }}>
            Clear
          </Button>
          <SelectorDialog visible={showSelectorDialog}
            submitSelector={(value) => {
              let toAdd = []
              skip: for (const v of value.values()) {
                for (const c of capturelist.values()) {
                  if (v.name == c.name) {
                    continue skip
                  }
                }
                toAdd = [...toAdd, v]
              }
              setCapturelist([...capturelist, ...toAdd])
              setShowSelectorDialog(!showSelectorDialog)
            }}
            onClose={() => { setShowSelectorDialog(!showSelectorDialog) }}></SelectorDialog>
        </div>
      </Form.Item>
      <br />
      <Form.Item label="Filter" >
        <Input name="filter" defaultValue={""} placeholder={"Filter expression, same as tcpdump."} style={{ width: 500 }} />
      </Form.Item>
      <br />
      <Form.Item label="Duration">
        <TimePicker name="duration" format="mm:ss" defaultValue={moment("00:30", "mm:ss", true)} />
      </Form.Item>
      <Form.Item>
        <Form.Submit type="primary" validate onClick={handleSubmit}>
          Submit Task
        </Form.Submit>
      </Form.Item>
    </Form>
  );
};

export default CaptureForm;
