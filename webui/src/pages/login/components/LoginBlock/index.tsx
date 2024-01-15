import { useState } from 'react';
import type { FunctionComponent } from 'react';
import { useAuth, history } from 'ice';
import { Input, Message, Form, Divider, Checkbox, Icon } from '@alifd/next';
import styles from './index.module.css';
import { fetchUserInfo, login } from '@/services/user';
import store from '@/store';
import { LoginParams, LoginResult } from '@/interfaces/user';
import Logo from '@/components/Logo';
import { getErrorMessage } from '@/utils';

const { Item } = Form;

const LoginMessage: FunctionComponent<{
  content: string;
}> = ({ content }) => {
  return (
    <Message
      type="error"
      visible
      closeable
      style={{
        marginBottom: 24,
      }}
    >
      {content}
    </Message>
  );
};

const DEFAULT_DATA: LoginParams = {
  username: '',
  password: '',
};

interface LoginProps {
  dataSource?: LoginParams;
}

const LoginBlock: FunctionComponent<LoginProps> = (
  props = { dataSource: DEFAULT_DATA },
): JSX.Element => {
  const { dataSource = DEFAULT_DATA } = props;

  const [, userDispatcher] = store.useModel('user');
  const [, setAuth] = useAuth();

  const [postData, setValue] = useState(dataSource);
  const [isRunning, checkRunning] = useState(false);
  const [loginResult, setLoginResult] = useState<LoginResult>({});

  const formChange = (values: LoginParams) => {
    setValue(values);
  };

  const sendCode = (values: LoginParams, errors: []) => {
    if (errors) {
      return;
    }
    // get values.phone
    checkRunning(true);
  };

  async function updateUserInfo() {
    const userInfo = await fetchUserInfo();
    userDispatcher.updateCurrentUser(userInfo);
  }

  const handleSubmit = async (values: LoginParams, errors: any) => {
    if (errors) {
      console.log('errors', errors);
      return;
    }
    try {
      const result = await login(values);
      if (result) {
        Message.success('Login succeed！');
        setAuth({
          login: true,
        });
        await updateUserInfo();
        const urlParams = new URL(window.location.href).searchParams;
        history?.push(urlParams.get('redirect') || '/');
        return;
      }
      setLoginResult(result);
    } catch (error) {
      Message.error(`Login failed：${getErrorMessage(error)}`);
    }
  };

  return (
    <div className={styles.loginBlock}>
      <div className={styles.innerBlock}>
        <Logo
          image='/logo.svg'
          imageStyle={{ height: 53, margin: "16px 0" }}
        />
        <Form value={postData} onChange={formChange} size="large">
          <>
            <Item required requiredMessage="Required">
              <Input name="username" maxLength={20} placeholder="Username" />
            </Item>
            <Item required requiredMessage="Required" style={{ marginBottom: 0 }}>
              <Input.Password name="password" htmlType="password" placeholder="Password" />
            </Item>
          </>
          <Item style={{ marginBottom: 10, marginTop: 20 }}>
            <Form.Submit
              htmlType="submit"
              type="primary"
              onClick={handleSubmit}
              className={styles.submitBtn}
              validate
            >
              Login
            </Form.Submit>
          </Item>
        </Form>
      </div>
    </div>
  );
};

export default LoginBlock;
