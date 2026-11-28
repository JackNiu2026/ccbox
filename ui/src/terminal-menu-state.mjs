export function terminalAccountSelection(accounts, preferred = "", policy = "ask", handoff) {
  const readyAccounts = accounts?.filter(account => account.status === "ready") || [];
  const available = readyAccounts.filter(account => account.enabled);
  const autoAvailable = !accounts?.length || available.length > 0;
  const manual = readyAccounts.find(account => account.id === preferred);
  const account = manual?.id || (preferred === "login" ? "login" : autoAvailable ? "auto" : readyAccounts[0]?.id || "login");
  const sources = account === "auto" ? available : readyAccounts.filter(item => item.id === account);
  const hasBackup = sources.some(source => source.handoff_group &&
    available.some(other => other.id !== source.id && other.handoff_group === source.handoff_group));
  const autoHandoff = hasBackup ? handoff ?? (account === "auto" && policy === "allow") : undefined;
  return { account, autoAvailable, readyAccounts, hasBackup, autoHandoff };
}
