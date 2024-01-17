import { history } from 'ice';
import { Avatar, Overlay, Menu, Icon } from '@alifd/next';
import styles from './index.module.css';
import store from '@/store';
import { logout } from '@/services/user';

const { Item } = Menu;
const { Popup } = Overlay;

export interface Props {
  name?: string;
}

const UserProfile = ({ name, avatar, mail }) => {
  return (
    <div className={styles.profile}>
      <div className={styles.avatar}>
        <Avatar src={avatar} alt="用户头像" />
      </div>
      <div className={styles.content}>
        <h4>{name}</h4>
        <span>{mail}</span>
      </div>
    </div>
  );
};

const HeaderAvatar = (props: Props) => {
  const {
    name = 'Admin',
  } = props;
  const [, userDispatcher] = store.useModel('user');

  const loginOut = async () => {
    await logout();
    const pathname = history?.location?.pathname;
    history?.push({
      pathname: '/login',
      search: pathname ? `redirect=${pathname}` : '',
    });
  };
  function onMenuItemClick(key: string) {
    if (key === 'logout') {
      userDispatcher.updateCurrentUser({});
      loginOut();
    }
  }

  return (
    <div className={styles.headerAvatar}>
      <span>{name}</span>
    </div>
  );
};

export default HeaderAvatar;
