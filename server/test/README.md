# 测试指南

本目录包含 Mdbrain 的后端测试与少量 App 测试入口。

## 目录

- [测试结构](#toc-structure)
- [运行后端测试](#toc-backend-tests)
- [运行指定测试](#toc-specific-tests)
- [运行 App 脚本测试（手动）](#toc-app-tests)

<a id="toc-structure"></a>
## 测试结构

```
server/test/
├── mdbrain/                # 后端测试（Clojure）
└── app/test.html           # App 脚本测试（手动）
```

<a id="toc-backend-tests"></a>
## 运行后端测试

在仓库根目录运行：

```bash
make backend-test
```

或直接运行：

```bash
cd server
clojure -M:test
```

<a id="toc-specific-tests"></a>
## 运行指定测试

```bash
cd server

clojure -X:test :patterns '["mdbrain.db-test"]'
clojure -X:test :patterns '["mdbrain.handlers.*"]'
```

<a id="toc-app-tests"></a>
## 运行 App 脚本测试（手动）

方式一：启动静态文件服务器。

```bash
cd server
python3 -m http.server 8000
open http://localhost:8000/test/app/test.html
```

方式二：直接打开文件。

```bash
open server/test/app/test.html
```
