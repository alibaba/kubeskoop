import { Dialog, Form, Input, Field } from "@alifd/next";
import { useState } from "react";

interface URLDialogProps {
  title?: string;
  visible: boolean;
  url?: string;
  onSubmit?: (f: URLDialogFields) => any;
  onVisibleChange: (c: boolean) => void;
}

interface URLDialogFields {
  url: string;
}

const URLDialog: React.FC<URLDialogProps> = (props: URLDialogProps): JSX.Element => {
  const { title = "URL" } = props
  const field = Field.useField({
    values: { 'url': props.url }
  });

  const [loading, setLoading] = useState(false);

  const submit = async () => {
    setLoading(true);
    const { errors } = await field.validatePromise();
    if (errors && errors.length > 0) {
      return;
    }

    if (props.onSubmit) {
        await props.onSubmit(field.getValues());
    };
    setLoading(false)
    props.onVisibleChange(false);
  };

  return (
    <Dialog
      v2
      title={title}
      visible={props.visible}
      okProps={{ loading }}
      onOk={submit}
      onClose={props.onVisibleChange?.bind(null, false)}
    >
      <Form field={field}>
        <Form.Item label="地址">
          <Input defaultValue={props.url} />
        </Form.Item>
      </Form>
    </Dialog>
  );
};

export default URLDialog;
