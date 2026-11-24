let messages = {};

export function setMessages(catalog) {
  messages = catalog;
}

export function translate(source, ...values) {
  const message = Object.hasOwn(messages, source) ? messages[source] : source;
  if (!values.length) return message;
  return message.replace(/\{(\d+)\}/g, (match, index) => Number(index) < values.length ? String(values[index]) : match);
}
