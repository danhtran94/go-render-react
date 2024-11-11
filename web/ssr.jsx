import React from "react";
import { renderToString } from "react-dom/server";

import App from "./App";

function render(props) {
  return renderToString(<App {...props} />);
}

globalThis.render = render;
