import React from "react";

function App(props) {
  return (
    <div>
      <h2>Golang Rendering React (SSR)</h2>
      <h3>using QuickJS + ESBuild</h3>
      <p>{props.message}</p>
    </div>
  );
}

export default App;
