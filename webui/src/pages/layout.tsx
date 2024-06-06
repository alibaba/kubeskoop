import { useState } from 'react';
import { Outlet, useLocation } from 'ice';
import { Shell, ConfigProvider } from '@alifd/next';
import store from '@/store';
import PageNav from '@/components/PageNav';
import HeaderAvatar from '@/components/HeaderAvatar';
import Logo from '@/components/Logo';
import enUS from '@alifd/next/lib/locale/en-us';


interface IGetDevice {
  (width: number): 'phone' | 'tablet' | 'desktop';
}
const getDevice: IGetDevice = (width) => {
  const isPhone =
    typeof navigator !== 'undefined' &&
    navigator &&
    navigator.userAgent.match(/phone/gi);

  if (width < 680 || isPhone) {
    return 'phone';
  } else if (width < 1280 && width > 680) {
    return 'tablet';
  } else {
    return 'desktop';
  }
};

export default function Layout() {
  const location = useLocation()
  const [device, setDevice] = useState(getDevice(NaN));

  if (typeof window !== 'undefined') {
    window.addEventListener('optimizedResize', (e) => {
      const deviceWidth =
        (e && e.target && (e.target as Window).innerWidth) || NaN;
      setDevice(getDevice(deviceWidth));
    });
  }

  const [userState] = store.useModel('user');

  if (['/login'].includes(location.pathname)) {
    return <Outlet />;
  }

  let contentStyle = {}
  if (['/monitoring/dashboard/pod', '/monitoring/dashboard/node', '/monitoring/flow', '/'].includes(location.pathname)) {
    contentStyle = {
      padding: "0px"
    }
  }

  return (
    <ConfigProvider device={device} locale={enUS}>
      <Shell
        style={{
          minHeight: '100vh',
        }}
        type="brand"
        fixedHeader={false}
      >
        <Shell.Branding>
          <Logo
            image="/header.svg"
          />
        </Shell.Branding>
        <Shell.Action>
          <HeaderAvatar
            name={userState.currentUser.user}
          />
        </Shell.Action>
        <Shell.Navigation collapse={false} trigger={null}>
          <PageNav />
        </Shell.Navigation>
        <Shell.Content style={contentStyle}>
          <Outlet />
        </Shell.Content>
      </Shell>
    </ConfigProvider>

  );
}
