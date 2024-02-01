import { Form, Input, Select, Radio, Checkbox, TimePicker, Button, Message } from '@alifd/next';
import { useEffect, useState } from "react";
import styles from "./index.module.css"
import moment from 'moment';
import k8sService from "@/services/k8s";
import SelectorDialog from "./selectorDialog.tsx"
import capture from '@/services/capture';

interface CaptureFormProps {
  onSubmit: (data: CaptureFormData) => void;
}

interface CaptureFormData {
  [key: string]: any;
}



const CaptureForm: React.FunctionComponent<CaptureFormProps> = (props: CaptureFormProps) => {
  const { onSubmit } = props;
  const [showSelectorDialog, setShowSelectorDialog] = useState(false)

  const [captureList, setCaptureList] = useState([])
  const handleSubmit = (values: CaptureFormData, errors: any) => {
    if (errors) {
      return
    }
    if (captureList.length == 0) {
      Message.error("Please select at least one target.")
      return
    }
    values["capture_list"] = captureList
    values["duration"] = values["duration"].minutes() * 60 + values["duration"].seconds()
    onSubmit(values);
    console.log(values)
  };

  return (
    <Form inline labelAlign='left'>
      <Form.Item label="Targets" >
        <div className={styles.custom}>
          {captureList.map((v, i) => {
            if (v.type == "Node") {
              return <Button className={styles.btn} key={i}>{v.type + ": " + v.name}</Button>;
            } else {
              return <Button className={styles.btn} key={i}>{v.type + ": " + v.namespace + "/" + v.name}</Button>;
            }
          })}
          <Button className={styles.btn} type="primary" onClick={() => { setShowSelectorDialog(!showSelectorDialog) }}>
            Add ＋{" "}
          </Button>
          <Button className={styles.btn} warning type="primary" onClick={() => { setCaptureList([]) }}>
            Clear
          </Button>
          <SelectorDialog visible={showSelectorDialog}
            submitSelector={(value) => {
              console.log(value)
              let toAdd = []
              skip: for (const v of value.values()) {
                if (v.type === 'Pod' && (!v.name || !v.namespace)) {
                  continue
                }
                if (v.type === 'Node' && !v.name) {
                  continue
                }

                for (const c of captureList.values()) {
                  if (v.name == c.name) {
                    continue skip
                  }
                }
                toAdd = [...toAdd, v]
              }
              setCaptureList([...captureList, ...toAdd])
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
