const { getESLintConfig } = require('@applint/spec');

module.exports = getESLintConfig('react-ts', {
  extends: ['@ali/eslint-config-att/typescript/react'],
});
