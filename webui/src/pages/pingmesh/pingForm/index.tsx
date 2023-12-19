import {Form, Input, Select, Radio, Checkbox, TimePicker, Button, Message} from '@alifd/next';
import {useEffect, useState} from "react";
import styles from "./index.module.css"
import moment from 'moment';
import k8sService from "@/services/k8s";
import SelectorDialog from "./selectorDialog.tsx"
import {PingMeshArgs} from "@/services/pingmesh";

interface PingFormProps {
  onSubmit: (data: PingMeshArgs) => void;
}

const PingForm: React.FunctionComponent<PingFormProps> = (props: PingFormProps) => {
  const { onSubmit } = props;
  const [showSelectorDialog, setshowSelectorDialog] = useState(false)

  const [pingMeshList, setPingMeshList] = useState([])
  const handleSubmit = (values: PingMeshArgs, errors: any) => {
    if (errors) {
      return
    }
    if(pingMeshList.length < 2) {
      Message.error("至少选择两个以上的对象用来做延迟探测")
      return
    }
    values.ping_mesh_list = pingMeshList
    onSubmit(values);
  };

  return (
    <Form inline labelAlign='left'>
      <Form.Item label="延迟探测对象列表" >
        <div className={styles.custom}>
          {pingMeshList.map((v, i) => {
            if(v.type == "Node") {
              return <Button className={styles.btn} key={i}>{v.type + ": " + v.name}</Button>;
            } else {
                return <Button className={styles.btn} key={i}>{v.type+": "+v.namespace+"/"+v.name}</Button>;
            }
          })}
          <Button className={styles.btn} type="primary" onClick={()=>{setshowSelectorDialog(!showSelectorDialog)}}>
            Add ＋{" "}
          </Button>
          <Button className={styles.btn} warning type="primary" onClick={()=>{setPingMeshList([])}}>
            清空
          </Button>
          <SelectorDialog visible={showSelectorDialog}
                          submitSelector={(value) => {
                            let toAdd = []
                            skip: for(const v of value.values()) {
                              for(const c of pingMeshList.values()) {
                                if(v.name == c.name) {
                                  continue skip
                                }
                              }
                              toAdd = [...toAdd, v]
                            }
                            setPingMeshList([...pingMeshList, ...toAdd])
                            setshowSelectorDialog(!showSelectorDialog)
                          }}
                          onClose={()=>{setshowSelectorDialog(!showSelectorDialog)}}></SelectorDialog>
        </div>
      </Form.Item>
      <br/>
      <Form.Item>
        <Form.Submit type="primary" validate onClick={handleSubmit}>
          发起探测任务
        </Form.Submit>
      </Form.Item>
    </Form>
  );
};

export default PingForm;
