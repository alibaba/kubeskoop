import { useEffect, useRef } from 'react';

export function useInterval(callback: () => void, delay: number) {
  const savedCallback = useRef(null);

  useEffect(() => {
    savedCallback.current = callback;
  });

  useEffect(() => {
    const tick = () => {
      savedCallback.current();
    };
    if (delay !== null) {
      const id = setInterval(tick, delay || 0);
      return () => clearInterval(id);
    }
  }, [delay]);
}
