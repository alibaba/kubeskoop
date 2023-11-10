import {Form, Input, Select, Radio, Checkbox, TimePicker, Button, Message} from '@alifd/next';
import {useEffect, useState} from "react";
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
  const [formCaptureType, setformCaptureType] = useState("Pod")
  const [formNamespace, setformNamespace] = useState("")
  const [showSelectorDialog, setshowSelectorDialog] = useState(false)

  const [capturelist, setCapturelist] = useState([])
  const handleSubmit = (values: CaptureFormData, errors: any) => {
    if (errors) {
      return
    }
    if(capturelist.length == 0) {
      Message.error("未选择抓包对象")
      return
    }
    values["capture_list"] = capturelist
    values["duration"] = values["duration"].minutes() * 60 + values["duration"].seconds()
    onSubmit(values);
    console.log(values)
  };

  return (
    <Form inline labelAlign='left'>
      <Form.Item label="抓包对象列表" >
        <div className={styles.custom}>
          {capturelist.map((v, i) => {
            if(v.type == "Node") {
              return <Button className={styles.btn} key={i}>{v.type + ": " + v.name}</Button>;
            } else {
                return <Button className={styles.btn} key={i}>{v.type+": "+v.namespace+"/"+v.name}</Button>;
            }
          })}
          <Button className={styles.btn} type="primary" onClick={()=>{setshowSelectorDialog(!showSelectorDialog)}}>
            Add ＋{" "}
          </Button>
          <Button className={styles.btn} warning type="primary" onClick={()=>{setCapturelist([])}}>
            清空
          </Button>
          <SelectorDialog visible={showSelectorDialog}
                          submitSelector={(value) => {
                            let toAdd = []
                            skip: for(const v of value.values()) {
                              for(const c of capturelist.values()) {
                                if(v.name == c.name) {
                                  continue skip
                                }
                              }
                              toAdd = [...toAdd, v]
                            }
                            setCapturelist([...capturelist, ...toAdd])
                            setshowSelectorDialog(!showSelectorDialog)
                          }}
                          onClose={()=>{setshowSelectorDialog(!showSelectorDialog)}}></SelectorDialog>
        </div>
      </Form.Item>
      <br/>
      <Form.Item label="抓包过滤条件" >
        <Input name="filter" defaultValue={""} placeholder={"抓包的条件，参考tcpdump的抓包命令文档"} style={{ width: 500 }} />
      </Form.Item>
      <br/>
      <Form.Item label="抓包持续时长">
        <TimePicker name="duration" format="mm:ss" defaultValue={moment("00:30", "mm:ss", true)} />
      </Form.Item>
      <Form.Item>
        <Form.Submit type="primary" validate onClick={handleSubmit}>
          发起抓包任务
        </Form.Submit>
      </Form.Item>
    </Form>
  );
};

export default CaptureForm;
