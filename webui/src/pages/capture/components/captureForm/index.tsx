import {Form, Input, Select, Radio, Checkbox, TimePicker} from '@alifd/next';
import {useState} from "react";
import moment from 'moment';

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
  const [formName, setformName] = useState("")
  const handleSubmit = (values: CaptureFormData, errors: any) => {
    if (errors) {
      return;
    }
    values["capture_type"] = formCaptureType
    values["name"] = values["name"].name
    if("namespace" in values) {
      values["namespace"] = values["namespace"].name
    }
    values["duration"] = values["duration"].minutes() * 60 + values["duration"].seconds()
    onSubmit(values);
  };
  const filterCaptureObject = () => formCaptureType == "Node"? nodes : pods.filter(item => item.namespace == formNamespace)

  return (
    <Form inline labelAlign='left'>
      <Form.Item label="抓包对象类型">
        <Radio.Group
          shape="button"
          value={formCaptureType}
          onChange={(value) => {setformCaptureType(value); setformNamespace(""); setformName("")}}
        >
          <Radio value="Node">Node</Radio>
          <Radio value="Pod">Pod</Radio>
        </Radio.Group>
      </Form.Item>
      {formCaptureType == "Pod" &&
        <Form.Item label="Namespace" required >
          <Select name="namespace" placeholder="请选择Namespace" dataSource={namespaces} useDetailValue
                  onChange={function (value) {setformNamespace(value.name); setformName("")}}
                  itemRender={(item) => `${item.name}`} valueRender={(item) => `${item.name}`} />
        </Form.Item>
      }
      <Form.Item label="Name" required>
        <Select name="name" placeholder="选择抓包的对象" useDetailValue
                value = {formName}
                dataSource={filterCaptureObject()}
                itemRender={(item) => `${item.name}`} valueRender={(item) => `${item.name}`}
                onChange={(value) => {setformName(value)}}
        />
      </Form.Item>
      {formCaptureType == "Pod" &&
        <Form.Item label="同时抓取Node空间" >
          <Checkbox name="capture_node" defaultValue={false} />
        </Form.Item>
      }
      <br/>
      <Form.Item label="抓包过滤条件" >
        <Input name="filter" defaultValue={""} placeholder={"抓包的条件，参考tcpdump的抓包命令文档"} style={{ width: 500 }} />
      </Form.Item>
      <br/>
      <Form.Item label="抓包持续时长">
        <TimePicker name="duration" format="mm:ss" defaultValue={moment("02:00", "mm:ss", true)} />
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


//fixme obtain pods from api
const pods = [
  {
    name: "nginx",
    namespace: "default",
    host: "cn-hangzhou.172.16.0.1"
  },
  {
    name: "redis",
    namespace: "default",
    host: "cn-hangzhou.172.16.0.2"
  }
];

const namespaces = [
  {
    name: "default"
  },
  {
    name: "kube-system"
  }
];

const nodes = [
  {
    name: "cn-hangzhou.172.16.0.1",
    ip: "172.16.0.1"
  },
  {
    name: "cn-hangzhou.172.16.0.2",
    ip: "172.16.0.2"
  }
]
