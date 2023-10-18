import * as React from 'react';

const EmptyBlock: React.FC = () => {
  return (
    <div className="table-empty-block">
      <div className="result-image">
        <img alt="data empty" src="//img.alicdn.com/tfs/TB1_yJXFkL0gK0jSZFAXXcA9pXa-1112-758.png" />
      </div>
      <div className="result-title">
        数据为空
      </div>
    </div>
  );
};

export default EmptyBlock;
