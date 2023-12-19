import {Form, Input, Select, Radio, Checkbox, TimePicker, Dialog, List, Divider} from '@alifd/next';
import React, {useEffect, useState} from "react";
import moment from 'moment';
import k8sService, {NodeInfo, PodInfo} from "@/services/k8s";
import styles from "./index.module.css"


interface SelectorProps {
  submitSelector: (data: []) => void;
  podList: PodInfo[];
  nodeList: NodeInfo[];
  visible: boolean;
  onClose: () => void
}

const SelectorDialog: React.FunctionComponent<SelectorProps> = (props: SelectorProps) => {
  const [nameList, setNameList] = useState([])
  const [namespaces, setNameSpaces] = useState([])
  const [formName, setformName] = useState("")
  const [formCaptureType, setformCaptureType] = useState("Pod")
  const [captureSelectorType, setcaptureSelectorType] = useState("Name")
  const [labelSelectorKeys, setLabelSelectorKeys] = useState([])
  const [labelSelectorKey, setLabelSelectorKey] = useState("")
  const [labelSelectorValues, setLabelSelectorValues] = useState([])
  const [labelSelectorValue, setLabelSelectorValue] = useState("")
  const [formNamespace, setformNamespace] = useState("")

  const filterCaptureObject = (type, ns) => {
    if (type == "Node") {
      k8sService.listNodes().then((res) => {
        setNameList(res)
      })
    } else {
      k8sService.listPods().then((res) => {
        setNameSpaces([...new Set(res.map(item => item.namespace))].map(item => ({name: item})))
        setNameList(res.filter(item => item.namespace == ns))
      })
    }
  }

  useEffect(() => {
    filterCaptureObject(formCaptureType, formNamespace)
  }, []);

  useEffect(() => {
    if(captureSelectorType == "Name") {
      setLabelSelectorKeys([])
    } else {
      let labelKeys = []
      nameList.forEach((item,idx, list) => {
        if(item.labels) {
          labelKeys = [...labelKeys,...Object.keys(item.labels)]
        }
      })

      setLabelSelectorKeys([...new Set(labelKeys)])
    }
    setLabelSelectorKey("")
  }, [formCaptureType, namespaces, captureSelectorType, formNamespace, nameList]);
  useEffect(() => {
    setLabelSelectorValue("")
    if(captureSelectorType == "Name") {
      setLabelSelectorValues([])
    } else {
      let values = nameList.map(item => {
        if(formCaptureType=="Pod") {
          if (item.namespace && item.namespace == formNamespace) {
            if (item.labels) {
              if (item.labels[labelSelectorKey]) {
                return item.labels[labelSelectorKey]
              }
            }
          }
        } else {
            if (item.labels) {
              if (item.labels[labelSelectorKey]) {
                return item.labels[labelSelectorKey]
              }
            }
        }
      })
      values = [...new Set(values)]
      setLabelSelectorValues(values)
    }
  }, [labelSelectorKeys, labelSelectorKey]);


  const selectedResult = () => {
      if(formCaptureType == "Pod") {
        if(captureSelectorType == "Name") {
          let capList = [
            {
              name: formName.name,
              namespace: formNamespace,
              nodename: formName.nodename,
              type: "Pod"
            }
          ]
          return capList
        } else {
          let selectPod = nameList.map((item) => {
            if(item.namespace && item.namespace == formNamespace && item.labels[labelSelectorKey] && item.labels[labelSelectorKey]==labelSelectorValue) {
              item.type = "Pod"
              return item
            }
          })
          selectPod = [...new Set(selectPod.filter(item => item))]
          return selectPod
        }
      } else if (formCaptureType == "Node") {
        if(captureSelectorType == "Name") {
          return [
            {
              name: formName.name,
              type: "Node"
            }
          ]
        } else {
          return [...new Set(nameList.map((item) => {
            if(item.labels[labelSelectorKey] && item.labels[labelSelectorKey]==labelSelectorValue) {
              item.type = "Node"
              return item
            }
          }).filter(item => item))]
        }
      }
      return []
  }

  return (
    <Dialog
      v2
      title="详情"
      footerActions={['ok']}
      visible={props.visible}
      onClose={props.onClose}
      onOk={()=>props.submitSelector(selectedResult())}
    >
    <Form inline labelAlign='left'>
      <Form.Item label="Ping对象类型">
        <Radio.Group
          shape="button"
          value={formCaptureType}
          onChange={(value) => {setformCaptureType(value); setformNamespace(""); setformName(""); filterCaptureObject(value)}}
        >
          <Radio value="Node">Node</Radio>
          <Radio value="Pod">Pod</Radio>
        </Radio.Group>
      </Form.Item>
      <br/>
      <Form.Item label="如何选择对象">
        <Radio.Group
          shape="button"
          value={captureSelectorType}
          onChange={(value) => {setcaptureSelectorType(value); setformNamespace(""); setformName(""); filterCaptureObject(formCaptureType)}}
        >
          <Radio value="Name">指定对象</Radio>
          <Radio value="Selector">LabelSelector</Radio>
        </Radio.Group>
      </Form.Item>

      {formCaptureType == "Pod" &&
        <Form.Item label="Namespace" required >
          <Select name="namespace" placeholder="请选择Namespace" dataSource={namespaces} useDetailValue showSearch
                  onChange={function (value) {setformNamespace(value.name); setformName(""); filterCaptureObject(formCaptureType, value.name);}}
                  itemRender={(item) => `${item.name}`} valueRender={(item) => `${item.name}`} />
        </Form.Item>
      }

      {captureSelectorType == "Selector" &&
        <Form.Item label="LabelSelector" required>
          <Select className={styles.selector} value={labelSelectorKey} onChange={setLabelSelectorKey} dataSource={labelSelectorKeys} style={{ width: 200 }} name="labelKey" placeholder="key"/>
          <span> = </span>
          <Select className={styles.selector} value={labelSelectorValue} onChange={setLabelSelectorValue} dataSource={labelSelectorValues} style={{ width: 200 }} name="labelVal" placeholder="value"/>
        </Form.Item>
      }
      {captureSelectorType == "Name" &&
      <Form.Item label="Name" required>
        <Select name="name" placeholder="选择互相ping的对象" useDetailValue showSearch
                value = {formName}
                dataSource={nameList}
                itemRender={(item) => `${item.name}`} valueRender={(item) => `${item.name}`}
                onChange={(value) => {setformName(value)}}
        />
      </Form.Item>
      }
    </Form>
    </Dialog>
  );
}

export default SelectorDialog;
