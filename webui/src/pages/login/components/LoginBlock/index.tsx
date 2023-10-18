import { useState } from 'react';
import type { FunctionComponent } from 'react';
import { useAuth, history } from 'ice';
import { Input, Message, Form, Divider, Checkbox, Icon } from '@alifd/next';
import { useInterval } from '@/hooks/useInterval';
import styles from './index.module.css';
import { fetchUserInfo, login } from '@/services/user';
import store from '@/store';
import { LoginParams, LoginResult } from '@/interfaces/user';
import Logo from '@/components/Logo';
import logo from '@/assets/logo.png';

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
  autoLogin: true,
  phone: '',
  code: '',
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
  const [isPhone, checkPhone] = useState(false);
  const [second, setSecond] = useState(59);
  const [loginResult, setLoginResult] = useState<LoginResult>({});

  useInterval(
    () => {
      setSecond(second - 1);
      if (second <= 0) {
        checkRunning(false);
        setSecond(59);
      }
    },
    isRunning ? 1000 : null,
  );

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
      if (result.success) {
        Message.success('登录成功！');
        setAuth({
          admin: result.userType === 'admin',
          user: result.userType === 'user',
        });
        await updateUserInfo();
        const urlParams = new URL(window.location.href).searchParams;
        history?.push(urlParams.get('redirect') || '/');
        return;
      }
      console.log(result);
      // 如果失败去设置用户错误信息，显示提示信息
      setLoginResult(result);
    } catch (error) {
      Message.error('登录失败，请重试！');
      console.log(error);
    }
  };

  const phoneForm = (
    <>
      <Item format="tel" required requiredMessage="必填" asterisk={false}>
        <Input
          name="phone"
          innerBefore={
            <span className={styles.innerBeforeInput}>
              +86
              <span className={styles.line} />
            </span>
          }
          maxLength={20}
          placeholder="手机号"
        />
      </Item>
      <Item required requiredMessage="必填" style={{ marginBottom: 0 }}>
        <Input
          name="code"
          innerAfter={
            <span className={styles.innerAfterInput}>
              <span className={styles.line} />
              <Form.Submit
                text
                type="primary"
                style={{ width: 64 }}
                disabled={!!isRunning}
                validate={['phone']}
                onClick={sendCode}
                className={styles.sendCode}
              >
                {isRunning ? `${second}秒后再试` : '获取验证码'}
              </Form.Submit>
            </span>
          }
          maxLength={20}
          placeholder="验证码"
        />
      </Item>
    </>
  );

  const accountForm = (
    <>
      <Item required requiredMessage="必填">
        <Input name="username" maxLength={20} placeholder="用户名: admin or user" />
      </Item>
      <Item required requiredMessage="必填" style={{ marginBottom: 0 }}>
        <Input.Password name="password" htmlType="password" placeholder="密码: ice" />
      </Item>
    </>
  );

  const byAccount = () => {
    checkPhone(false);
  };

  const byForm = () => {
    checkPhone(true);
  };

  return (
    <div className={styles.loginBlock}>
      <div className={styles.innerBlock}>
        <Logo
          image={logo}
          text="ICE Pro"
          imageStyle={{ height: 48 }}
          textStyle={{ color: '#000', fontSize: 24 }}
        />
        <div className={styles.desc}>
          <span onClick={byAccount} className={isPhone ? undefined : styles.active}>
            账户密码登录
          </span>
          <Divider direction="ver" />
          <span onClick={byForm} className={isPhone ? styles.active : undefined}>
            手机号登录
          </span>
        </div>
        {loginResult.success === false && (
          <LoginMessage
            content="账户或密码错误(admin/ice)"
          />
        )}
        <Form value={postData} onChange={formChange} size="large">
          {isPhone ? phoneForm : accountForm}

          <div className={styles.infoLine}>
            <Item style={{ marginBottom: 0 }}>
              <Checkbox name="autoLogin" className={styles.infoLeft}>
                自动登录
              </Checkbox>
            </Item>
            <div>
              <a href="/" className={styles.link}>
                忘记密码
              </a>
            </div>
          </div>

          <Item style={{ marginBottom: 10 }}>
            <Form.Submit
              htmlType="submit"
              type="primary"
              onClick={handleSubmit}
              className={styles.submitBtn}
              validate
            >
              登录
            </Form.Submit>
          </Item>
          <div className={styles.infoLine}>
            <div className={styles.infoLeft}>
              其他登录方式 <Icon type="atm" size="small" /> <Icon type="atm" size="small" />{' '}
              <Icon type="atm" size="small" />
            </div>
            <a href="/" className={styles.link}>
              注册账号
            </a>
          </div>
        </Form>
      </div>
    </div>
  );
};

export default LoginBlock;
