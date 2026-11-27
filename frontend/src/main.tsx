import React from "react";
import ReactDOM from "react-dom/client";
import { ConfigProvider, theme } from "antd";
import App from "./App";
import "./styles.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ConfigProvider
      theme={{
        algorithm: theme.darkAlgorithm,
        token: {
          colorPrimary: "#6d8cff",
          colorBgBase: "#10151f",
          colorBgContainer: "#171e2b",
          borderRadius: 12,
          fontFamily: 'Inter, "PingFang SC", "Microsoft YaHei", sans-serif'
        }
      }}
    >
      <App />
    </ConfigProvider>
  </React.StrictMode>
);
