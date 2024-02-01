import { Form, Input, Select, Radio, Checkbox, TimePicker, Dialog, List, Divider, Box } from '@alifd/next';
import React, { useEffect, useState } from "react";
import moment from 'moment';
import k8sService, { NodeInfo, PodInfo } from "@/services/k8s";
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
  const [formName, setFormName] = useState("")
  const [formCaptureType, setFormCaptureType] = useState("Pod")
  const [captureSelectorType, setCaptureSelectorType] = useState("Name")
  const [labelSelectorKeys, setLabelSelectorKeys] = useState([])
  const [labelSelectorKey, setLabelSelectorKey] = useState("")
  const [labelSelectorValues, setLabelSelectorValues] = useState([])
  const [labelSelectorValue, setLabelSelectorValue] = useState("")
  const [formNamespace, setFormNamespace] = useState("")
  const [capturePodsNodes, setCapturePodsNodes] = useState(false)

  const filterCaptureObject = (type, ns) => {
    if (type == "Node") {
      k8sService.listNodes().then((res) => {
        setNameList(res)
      })
    } else {
      k8sService.listPods().then((res) => {
        setNameSpaces([...new Set(res.map(item => item.namespace))].map(item => ({ name: item })))
        setNameList(res.filter(item => item.namespace == ns))
      })
    }
  }

  useEffect(() => {
    filterCaptureObject(formCaptureType, formNamespace)
  }, []);

  useEffect(() => {
    if (captureSelectorType == "Name") {
      setLabelSelectorKeys([])
    } else {
      let labelKeys = []
      nameList.forEach((item, idx, list) => {
        if (item.labels) {
          labelKeys = [...labelKeys, ...Object.keys(item.labels)]
        }
      })

      setLabelSelectorKeys([...new Set(labelKeys)])
    }
    setLabelSelectorKey("")
  }, [formCaptureType, namespaces, captureSelectorType, formNamespace, nameList]);
  useEffect(() => {
    setLabelSelectorValue("")
    if (captureSelectorType == "Name") {
      setLabelSelectorValues([])
    } else {
      let values = nameList.map(item => {
        if (formCaptureType == "Pod") {
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
    if (formCaptureType == "Pod") {
      if (captureSelectorType == "Name") {
        let capList = [
          {
            name: formName.name,
            namespace: formNamespace,
            nodename: formName.nodename,
            type: "Pod"
          }
        ]
        if (capturePodsNodes) {
          capList = [...capList, { type: "Node", name: formName.nodename }]
        }
        return capList
      } else {
        let selectPod = nameList.filter(item => {return item.namespace && item.namespace == formNamespace && item.labels[labelSelectorKey] && item.labels[labelSelectorKey] == labelSelectorValue})
                                .map(i => ({type: "Pod", ...i}))
        selectPod = [...new Set(selectPod.filter(item => item))]

        if (capturePodsNodes) {
          const selectNodeNames = new Set(selectPod.filter(i => i).map(i => i.nodename));
          const selectNode = Array.from(selectNodeNames).map(i => ({ type: "Node", name: i }))
          console.log(selectNode)
          selectPod = [...selectPod, ...selectNode]
        }
        return selectPod
      }
    } else if (formCaptureType == "Node") {
      if (captureSelectorType == "Name") {
        return [
          {
            name: formName.name,
            type: "Node"
          }
        ]
      } else {
        return [...new Set(nameList.map((item) => {
          if (item.labels[labelSelectorKey] && item.labels[labelSelectorKey] == labelSelectorValue) {
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
      title="Add Target"
      footerActions={['ok']}
      visible={props.visible}
      onClose={props.onClose}
      onOk={() => props.submitSelector(selectedResult())}
    >
      <Form
        labelAlign='left'
        labelCol={{ fixedSpan: 6 }}
        wrapperCol={{ span: 16 }}
      >
        <Form.Item label="Type">
          <Radio.Group
            shape="button"
            value={formCaptureType}
            onChange={(value) => { setFormCaptureType(value); setFormNamespace(""); setFormName(""); filterCaptureObject(value) }}
          >
            <Radio value="Node">Node</Radio>
            <Radio value="Pod">Pod</Radio>
          </Radio.Group>
        </Form.Item>
        <Form.Item label="Select Target By">
          <Radio.Group
            shape="button"
            value={captureSelectorType}
            onChange={(value) => { setCaptureSelectorType(value); setFormNamespace(""); setFormName(""); filterCaptureObject(formCaptureType) }}
          >
            <Radio value="Name">Namespace & Name</Radio>
            <Radio value="Selector">Label Selector</Radio>
          </Radio.Group>
        </Form.Item>

        {formCaptureType == "Pod" &&
          <Form.Item label="Namespace" required >
            <Select className={styles.selector} name="namespace" placeholder="Please select namespace" dataSource={namespaces} useDetailValue showSearch
              onChange={function (value) { setFormNamespace(value.name); setFormName(""); filterCaptureObject(formCaptureType, value.name); }}
              itemRender={(item) => `${item.name}`} valueRender={(item) => `${item.name}`} filter={(k, i) => i.name.startsWith(k)} />
          </Form.Item>
        }

        {captureSelectorType == "Selector" &&
          <Form.Item label="LabelSelector" required>
            <Box direction="row" style={{ alignItems: "center" }}>
              <Select showSearch value={labelSelectorKey} onChange={setLabelSelectorKey} dataSource={labelSelectorKeys} style={{ width: 200 }} name="labelKey" placeholder="key" />
              <span style={{ margin: "0 10px" }}>=</span>
              <Select showSearch value={labelSelectorValue} onChange={setLabelSelectorValue} dataSource={labelSelectorValues} style={{ width: 200 }} name="labelVal" placeholder="value" />
            </Box>
          </Form.Item>
        }
        {captureSelectorType == "Name" &&
          <Form.Item label="Name" required>
            <Select className={styles.selector} name="name" placeholder="Please select name" useDetailValue showSearch
              value={formName}
              dataSource={nameList}
              itemRender={(item) => `${item.name}`} valueRender={(item) => `${item.name}`}
              onChange={(value) => { setFormName(value) }}
              filter={(k, i) => i.name.startsWith(k)}
            />
          </Form.Item>
        }
        {formCaptureType == "Pod" &&
          <Form.Item label=" ">
            <Checkbox label="Also capture node packets" checked={capturePodsNodes} onChange={(value) => { setCapturePodsNodes(!capturePodsNodes); }} />
          </Form.Item>
        }
      </Form>
    </Dialog>
  );
}

export default SelectorDialog;
