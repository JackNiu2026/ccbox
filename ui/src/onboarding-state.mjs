// Starting always requires a node applied by the CCBox proxy manager.
export function needsProxySetup(_settings, _status, proxy) {
  return !(proxy?.active && proxy?.selected);
}
