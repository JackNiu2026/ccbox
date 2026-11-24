import React, { useState } from "react";
import { languages, selectInterfaceLanguage, translate, useInterfaceLanguage } from "./interface-language.mjs";

export function InterfaceLanguageControl() {
  const language = useInterfaceLanguage();
  const [error, setError] = useState("");
  return <label>
    {translate("界面语言")}
    <select value={language} onChange={(event) => {
      setError("");
      void selectInterfaceLanguage(event.target.value).catch((failure) => setError(failure.message));
    }}>
      {languages.map(({ id, name }) => <option key={id} value={id}>{name}</option>)}
    </select>
    {error && <small role="alert">{error}</small>}
  </label>;
}
