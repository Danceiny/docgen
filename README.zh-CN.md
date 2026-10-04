# docgen

**从 Go 源码生成 OpenAPI 3.0 文档。** docgen 用 `go/packages` 读取你的包，找出服务和它们收发的类型，写出 YAML。
不往你的代码里生成任何东西，也不执行任何代码：文档来自你本来就在写的类型、struct tag 和注释。

[English](README.md)

```go
// Get returns one pet.
//
// @desc: Looks a pet up by its id.
// @tags: public
// @response:404,NotFoundErr,There is no pet with that id
func (PetService) Get(ctx context.Context, req *protocol.GetPetReq) (*domain.Pet, error) {
```

会变成 `POST /api/pet/get`：有请求体、schema 为 `domain.Pet` 的 `200` 响应、对应 `NotFoundErr` 的 `404` 响应，以及 tag。

- **一个模块，多份文档。** 同一份源码生成内部文档（全部内容）和公开文档（你挑选的接口和字段）。
- **CI 校验模式。** `docgen -check` 在入库的文档与源码生成结果不一致时失败，并指出差异位置。
- **配置即数据。** 一个 `docgen.yaml`，外加可选的 overlay 和错误目录文件。没有要编译进去的插件 API。
- **确定性。** 同样的源码生成同样的字节，文档可以放进 PR 里评审、做 diff。

运行 docgen 的机器需要 Go 1.25 或更高版本；被读取的模块可以面向该机器能构建的任何 Go 版本。

## 安装

```sh
go install github.com/Danceiny/docgen/cmd/docgen@latest
```

或者把它作为 tool 固定在你的模块里（Go 1.24 及以上），让所有人和 CI 用同一个版本：

```sh
go get -tool github.com/Danceiny/docgen/cmd/docgen@v0.1.0
go tool docgen -version
```

## 快速开始

[`examples/petstore`](examples/petstore) 是一个小模块，用到了 docgen 的大部分能力，两份文档都已入库。

```sh
git clone https://github.com/Danceiny/docgen
cd docgen/examples/petstore
docgen            # 写出 docs/api/internal.yaml 和 docs/api/public.yaml
docgen -check     # 任一文件与源码生成结果不同则以 1 退出
```

给自己的模块写文档：在模块根目录写一个 `docgen.yaml`：

```yaml
version: 1
docs:
  - name: api
    audience: internal
    output: docs/api/openapi.yaml
    info: {title: My API, version: 1.0.0}
    models: ["*/domain", "*/protocol"]   # 其类型成为 schema 的包
    services: ["*/service"]              # 其服务成为接口的包
```

然后在该目录运行 `docgen`。模式匹配的是相对于模块路径的包路径；`*` 匹配任意一段字符（包括斜杠），其余字符只匹配自身。

## docgen 找什么

### 服务与接口

**服务**是带有 `Name() string` 方法的 struct，该方法返回字符串字面量或常量。服务的导出方法就是**接口**，
但 `Name` 本身、标了 `@apidoc: -` 的方法、以及列在 `api.skip_methods` 里的方法除外。

```go
type PetService struct{}

func (PetService) Name() string { return "pet" }

func (PetService) Get(ctx context.Context, req *protocol.GetPetReq) (*domain.Pet, error)
```

- 第一个参数可以是 `context.Context`，它不进文档。
- struct（或 struct 指针）参数是**请求体**。其他类型的参数用 `@param` 注解描述。
- 不是 `error` 的返回值是 **`200` 响应**。`error` 本身不进文档，用 `@response`（见下）。
- 路由是 `<前缀>/<服务名>/<方法名首字母小写>`，所以服务 `pet` 的 `Get` 是 `/api/pet/get`。
  服务名可以含斜杠（`store/order`）。前缀默认 `/api`，可用 `api.prefix` 修改。
- 默认 HTTP 方法是 `POST`，用 `@method` 修改。
- 接口的 tag 依次是：首字母大写的服务名、`@tags`、`@permission`。
- summary 是注释的第一行，description 来自 `@desc`。

[`route`](route) 包是路由规则的可执行规格。如果你有自己的路由器，用 `route.Resolve` 测一测它。

### 方法上的注解

写在方法的注释里，一行一个。

| 注解 | 含义 |
|---|---|
| `@desc: text` | 接口的描述，直到下一个注解为止，可以多段、可以用 Markdown。 |
| `@tags: a, b` | 额外的 tag。 |
| `@permission: pet:write` | 也作为 tag 加上，方便看出调用方需要什么。 |
| `@method: GET` | HTTP 方法，逗号分隔：`GET`、`POST`、`PUT`、`PATCH`、`DELETE`、`HEAD`、`OPTIONS`、`TRACE`。 |
| `@path: /custom` | 替换路由中的方法名。值本身已以前缀开头时，它就是完整路由。 |
| `@doc: Guide: https://example.com/guide` | 外部文档，形式为 `描述: url`。 |
| `@response:404,NotFoundErr,text` | 额外的响应：状态码、[错误目录](#错误目录)中某个错误的名字、描述。状态码会与目录核对。 |
| `@param:query limit int required "how many"` | 不是请求体的参数：`<位置> <名字> <类型> <required\|optional> ["描述"]`，位置为 `query`、`header`、`path` 或 `cookie`。 |
| `@headerType: Admin` | 在 `headers.types` 里为该接口选择 header 类型。 |
| `@apidoc: -` | 该方法不是接口：既不路由（按 `route.Skipped` 的约定），也不进文档。 |

### 类型

`models` 包里的具名类型成为**组件**（`#/components/schemas/<点号化的 import 路径>.<Name>`）。

- 字段名来自 `json` tag。`json:"-"` 和未导出字段被略去。内嵌 struct 会被展开。
- 字段带 `validate:"required"`、`binding:"required"` 或 `required:"true"` 时为必填。`json:"x,nullable"` 表示可为 null。
- `example:"..."` 和 `default:"..."` tag（或 `json:"x,default=..."`）给出字段的示例和默认值。
- 类型或字段的注释就是它的描述。
- 为具名类型声明的常量使它成为**枚举**：值和注释列在 `enum` 与描述里。
- 直接或经由其他类型包含自身的类型，表现为指向自身的 `$ref`。
- `time.Time` 是 `date-time` 字符串，`time.Duration` 是 `int64` 纳秒数，与 JSON 的写法一致。要改用 `type_map`。

### 谁能看到什么

文档有一个**受众**，`internal` 或 `public`，可见性规则据此决定显示什么。

**类型**上，写在注释里：

```go
//apidoc:public                    在公开文档中显示，因此内部文档中也显示
//apidoc:internal                  仅在内部文档中显示
//apidoc:hidden                    完全不显示
//apidoc:public:Active,Pending     枚举：只显示这些值
//apidoc:public:-Legacy*           枚举：除以 Legacy 开头的值外都显示
```

**字段**上，写在 struct tag 里：

```go
Notes string `json:"notes" apidoc:"internal"`   // 仅内部文档
Beta  string `json:"beta"  apidoc:"public"`     // 仅公开文档
Cost  int    `json:"cost"  apidoc:"hidden"`     // 完全不显示（`apidoc:"-"` 同）
```

公开文档还会去掉名字以 `hide_type_prefixes` 中某个前缀开头的类型，除非它自己的 `//apidoc:` 另有说明。没有注解的内容在所有文档里都显示。

旧形式 `apidoc:"Staff"` 只在 `legacy_field_tokens` 里列了 `Staff` 的文档中显示该字段。
tag `api.header:"X-Request-Id"` 给 header 类型的字段指定它所用的 HTTP header 名。

## 配置

`docgen.yaml` 是严格的：未知的键、非法的值、无法工作的模式都是指明键名的错误，并且所有问题会一次性报告。
[petstore 的配置](examples/petstore/docgen.yaml)是完整的例子，下面是参考：

```yaml
version: 1                      # 唯一的版本

api:
  prefix: /api                  # 每条路由的前缀；默认 /api
  skip_methods: [GetChildren]   # 除 Name 外永远不是接口的方法

vendor_extensions: false        # 添加 x-apifox-*、x-enum-varnames、x-enum-comments、
                                # x-display-name、x-primary-property、x-go-interface
keep_empty_tags: false          # 保留没有 @tags 的接口上的空 tag

type_map:                       # 直接指定某个类型的 schema，不去读它
  example.com.shop.types.ID: {type: string, description: id}
  github.com.shopspring.decimal.Decimal: {type: number}
  time.Duration: {type: string, format: duration, example: 1h30m}

headers:                        # 每个接口作为 "Headers" 参数所带的 header 类型
  default: Auth
  types: {Auth: example.com.shop.protocol.AuthHeader}

request:
  multipart:                    # multipart/form-data 的请求类型
    example.com.shop.protocol.UploadReq:
      - {name: file, kind: file, required: true}
      - {name: caption, kind: scalar, scalar_to: string}
  query:                        # 从 URL query 读取的请求类型：没有请求体
    example.com.shop.protocol.ListReq:
      - {name: status, type: string, description: Limit the list}
  runtime_only:                 # 任何 JSON 值都无法满足的绑定
    - {path: /api/shop/setHook, type: example.com.shop.Hook}

response:
  envelope: {code: code, message: message, data: data}   # 给 data 套一层信封；没有的话 data 就是响应体
  default_statuses: {"401": Unauthorized}                # 加到每个接口上
  binary:                       # 是文件的响应类型
    example.com.shop.protocol.DownloadResp:
      description: The file
      content_types: [image/png]
      errors: {"404": Not found}

errors:                         # @response 引用的错误目录
  file: errors.json
  component_prefix: example.com.shop.errors.

generic_titles: []              # 其 oneOf 不按类型参数收窄的泛型类型的完整 key

docs:
  - name: internal              # 用 -doc 选择文档
    audience: internal          # internal | public
    output: docs/api/internal.yaml
    info: {title: Shop API, version: 1.0.0, description: ...}
    servers: [{url: "http://localhost:8080", description: Local}]
    models: ["*/domain", "*/protocol"]
    services: ["*/service"]
    legacy_field_tokens: []     # 在此文档中显示的 apidoc:"..." tag 值
    hide_type_prefixes: []      # 名字以这些前缀开头的类型被略去
    overlay:                    # 手写的 schema，见 Overlay
      - {file: overlay.yaml, stage: after_models}

  - name: public
    audience: public
    output: docs/api/public.yaml
    info: {title: Shop API, version: 1.0.0}
    models: ["*/domain", "*/protocol"]
    services: ["*/service"]
    force_keep: []              # 即使没有接口使用也保留的组件
    public:                     # 仅 audience: public 可用
      tag: public               # 带此 tag 的接口是公开的
      strip_tags_containing: [] # 去掉包含这些字符串的 tag
      errors_last: false        # oneOf/anyOf/allOf 中把错误分支排在最后
```

`models` 必须包含每个接口收发的类型所在的包，否则运行会因没有 schema 的引用而失败。docgen 加载 `./...`，
模块里任何包无法构建时都会失败，并说明原因。

### 错误目录

`errors.json` 列出服务可能返回的错误：

```json
[
  {"name": "NotFoundErr", "code": 40401, "message": "not found", "httpCode": 404}
]
```

每个错误成为一个组件（名字由 `errors.component_prefix` 决定），`@response:404,NotFoundErr,text` 按名字引用它。
注解里的状态码与该错误的 `httpCode` 不一致时，docgen 会警告并采用错误目录里的值。
这个文件可以用几行 Go 从你自己的错误定义生成，并和文档一起入库。

### Overlay

有些 schema 无法从 Go 里读出来：声明在文档范围之外的类型、webhook 的载荷、手写的联合类型。
overlay 文件把它们加进来，或替换生成的内容：

```yaml
version: 1
schemas:
  example.com.shop.protocol.ImportReq:
    type: object
    required: [source]
    properties:
      source: {type: string, description: Where the pets come from}
      pets:
        type: array
        items: {$ref: '#/components/schemas/example.com.shop.domain.Pet'}
```

`after_models` 的 overlay 在生成接口之前应用，这使接口可以引用只存在于 overlay 里的 schema；
`after_apis` 的 overlay 在之后应用。overlay 只认 OpenAPI 3.0 文档用到的那部分 JSON Schema，其余一律拒绝。

## 命令行

```
docgen [-C dir] [-config file] [-doc name]... [-check] [-v]
docgen -version
```

| 参数 | |
|---|---|
| `-C dir` | 像在 `dir` 中启动一样运行，`dir` 是要生成文档的模块根目录。 |
| `-config file` | 配置文件，相对于 `dir`；默认 `docgen.yaml`。 |
| `-doc name` | 只生成该名字的文档；可重复。 |
| `-check` | 生成到临时目录，与磁盘上的文件比较，有不同则以 1 退出。不写任何文件。 |
| `-v` | 额外记录有助于跟踪一次运行的信息，例如被注解隐藏的每个类型。 |
| `-version` | 打印版本。 |

警告输出到标准错误；与代码有关的警告会指出位置。

在 CI 里：

```sh
go tool docgen -check
```

## 已知限制

docgen 还很年轻，它最初服务的那个项目的文档决定了它的很多行为。以下限制已知，后续版本会改变，请不要依赖它们。

- **map 丢失值类型**：`map[string]Pet` 是 `additionalProperties: true`。
- **枚举从字面量读取。** 用 `iota` 或表达式声明的常量值为空；隐式重复类型的常量块里只列出第一个常量。
- `[]byte` 是字符串数组（JSON 写的是 base64 字符串），`byte` 是字符串，`json:",string"` 被忽略。
- **其他模块的类型就地展开**，作为内联 schema 而不是组件；其中包含自身的类型第二次出现时只是一个 `object`。
  不要把它们用作接口的请求或响应类型：用你模块里的类型包一层。
- 带描述的引用会就地展开，而不是带同级字段的 `$ref`（OpenAPI 3.0 不允许）；字段的描述被保留，schema 被复制。
  从未被填充的 schema 上可能出现标记 `pattern: default`。
- **泛型按声明文档化**：类型参数类型的字段没有 schema，有多个类型参数的泛型类型不受支持。
- 文档是 OpenAPI 3.0；kin-openapi 不写 3.1。
- 接口的请求和响应类型必须声明在匹配 `models` 的包里。

## 兼容性

1.0 之前，命令行、配置和输出都可能在次版本中变化。变化记录在 [CHANGELOG.md](CHANGELOG.md)，同一版本内的输出不变。
请固定版本，并把文档入库。

唯一可以 import 的 Go 包是没有任何依赖的 [`route`](route)，其余都是 internal。

## 贡献

见 [CONTRIBUTING.md](CONTRIBUTING.md)。简言之：`go test ./...` 必须通过，
`go run ./cmd/docgen -C examples/petstore -check` 必须无输出。

## 许可证

[MIT](LICENSE)。docgen 所依赖的第三方代码列在 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
