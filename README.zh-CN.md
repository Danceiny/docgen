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
所以 `*/service` 匹配 `pet/service` 和 `shop/pet/service`，但不匹配模块根目录下的包 `service`：扁平的目录结构要写
`services: ["service"]`（或两个都写）。没有匹配到任何包的模式会被报告，否则它所属的文档会悄悄地变空。

## docgen 找什么

### 服务与接口

**服务**是带有 `Name() string` 方法的 struct，该方法返回字符串常量：字面量、模块内任何包里的具名常量或常量表达式。
服务的导出方法就是**接口**，但 `Name` 本身、标了 `@apidoc: -` 的方法、以及列在 `api.skip_methods` 里的方法除外。

```go
type PetService struct{}

func (PetService) Name() string { return "pet" }

func (PetService) Get(ctx context.Context, req *protocol.GetPetReq) (*domain.Pet, error)
```

- 第一个参数可以是 `context.Context`，它不进文档。
- 接下来的参数是**请求**：struct（或 struct 指针）是 JSON 请求体，列表和 map 也是；基本类型的参数是该类型的请求体，
  除非 `@param` 注解说明它是 query、header、path 或 cookie 参数。只读取 context 之后的第一个参数：
  方法有更多参数时 docgen 会警告，因为其余的参数在文档里没有位置。
- 不是 `error` 的返回值是 **`200` 响应**：struct 是指向其组件的引用，列表是元素类型的 `array`，map 是 `object`，
  `Page[Pet]` 这样实例化的泛型类型是 `Page` 的组件。`error` 本身不进文档，用 `@response`（见下）。
- 路由是 `<前缀>/<服务名>/<方法名首字母小写>`，所以服务 `pet` 的 `Get` 是 `/api/pet/get`。
  服务名可以含斜杠（`store/order`）。前缀默认 `/api`，可用 `api.prefix` 修改。`@path` 替换方法名（见下），
  以服务名开头的 `@path` 会被去掉这个前缀，按纯前缀处理：服务 `order` 的 `@path: /orderList` 是 `/api/order/list`，
  `@path: /order/list` 也是；方法名本身从不被去掉前缀（`OrderList` 是 `/api/order/orderList`）。
- 默认 HTTP 方法是 `POST`，用 `@method` 修改；写了多个方法（`@method: GET, POST`）时每个方法一个接口，
  id 上加小写的方法名（`pet/get_get`、`pet/get_post`）。OpenAPI 不允许 `GET` 和 `DELETE` 带请求体，
  但 docgen 仍把这类接口的请求写成请求体，除非该请求类型列在 `request.query` 里，那样就变成 query 参数（见配置）。
- 接口的 tag 是服务名、`@tags` 里的和 `@permission` 里的，去重并排序。服务名每个单词的首字母大写、其余保持原样
  （`myService/Sub` 是 `MyService/Sub`）；`@tags` 和 `@permission` 里的 tag 除每个单词的首字母外都变成小写
  （`OAuth` 是 `Oauth`，`x-y` 是 `X-Y`）。
- summary 是注释的第一行，description 来自 `@desc`。

[`route`](route) 包是路由规则的可执行规格。如果你有自己的路由器，用 `route.Resolve` 测一测它。

### 方法上的注解

写在方法的注释里，一行一个。docgen 不认识的注解会被放过，因为别的工具也会读注释；
与已知注解只差一个字母的（`@respone`、`@Tags`）会被报告。

| 注解 | 含义 |
|---|---|
| `@desc: text` | 接口的描述，直到下一个注解为止，可以多段、可以用 Markdown。 |
| `@tags: a, b` | 额外的 tag。 |
| `@permission: pet:write` | 也作为 tag 加上，方便看出调用方需要什么。 |
| `@method: GET` | HTTP 方法，逗号分隔：`GET`、`POST`、`PUT`、`PATCH`、`DELETE`、`HEAD`、`OPTIONS`、`TRACE`。 |
| `@path: /custom` | 替换路由中的方法名。值本身已以前缀开头时，它就是完整路由。 |
| `@doc: Guide: https://example.com/guide` | 外部文档，形式为 `描述: url`。 |
| `@response:404,NotFoundErr,text` | 额外的响应：状态码、[错误目录](#错误目录)中某个错误的名字、描述。状态码会与目录核对。 |
| `@param:query limit int required "how many"` | 不是请求体的参数：`<位置> <名字> <类型> <required\|optional> ["描述"]`，位置为 `query`、`header`、`path` 或 `cookie`。名字是 Go 参数的名字（或按位置写 `param1`、`param2`……），也是文档里的名字。`path` 参数必须出现在 `@path` 里，如 `/get/{id}`。 |
| `@headerType: Admin` | 在 `headers.types` 里为该接口选择 header 类型。 |
| `@apidoc: -` | 该方法不是接口：既不路由（按 `route.Skipped` 的约定），也不进文档。 |

### 类型

`models` 包里的具名类型成为**组件**（`#/components/schemas/<点号化的 import 路径>.<Name>`）。

- 字段按 `encoding/json` 的方式读取。名字来自 `json` tag；`json:"-"` 和未导出字段被略去；一次声明多个名字
  （`Lat, Lng float64`）每个名字都是一个字段，空白字段不是字段。内嵌 struct 会被展开到外层 struct 里，
  除非它带有 json 名字（``Base `json:"base"` ``），那样它就是该名字的字段。
- 字段的 `validate` 或 `binding` tag 含有规则 `required`（`validate:"required,email"` 含有），或带 `required:"true"` 时为必填。
  `json:"x,nullable"` 表示可为 null。
- `example:"..."` 和 `default:"..."` tag（或 `json:"x,default=..."`）给出字段的示例和默认值。
- **字段**的注释（在字段上方或行尾）就是它的描述；类型的注释不会用到。类型是组件的字段是 `$ref`，
  而 OpenAPI 3.0 不允许 `$ref` 旁边有描述，所以内部文档会略去这种字段的注释。
- 为具名类型声明的常量使它成为**枚举**，不论常量怎样声明（`iota`、移位、表达式）：编译器算出的值列在 `enum` 里，
  常量的名字和注释列在描述里。类型至少有一个常量用该类型声明（`A Status = iota`）才是枚举；
  `type Status = string` 只是 `string` 的另一个名字，只有用这个名字声明的常量才让它成为枚举。
- map 是 `object`，其 `additionalProperties` 是值的 schema（任意值的 map 是 `true`），`any` 和 `interface{}` 是空 schema，
  字节切片是 base64 `string`，byte 是数字。
- 直接或经由其他类型包含自身的类型，表现为指向自身的 `$ref`。
- `time.Time` 是 `date-time` 字符串，`time.Duration` 是 `int64` 纳秒数，与 JSON 的写法一致。要改用 `type_map`。

### 谁能看到什么

文档有一个**受众**，`internal` 或 `public`，可见性规则据此决定显示什么。

**类型**上，写在注释里：

```go
//apidoc:public                           在公开文档中显示，因此内部文档中也显示
//apidoc:internal                         仅在内部文档中显示
//apidoc:hidden                           完全不显示
//apidoc:public:StatusActive,StatusNew    枚举：只显示这些值
//apidoc:public:-StatusLegacy*            枚举：除以 StatusLegacy 开头的值外都显示
```

斜杠后面不能有空格（`// apidoc:hidden` 只是普通注释），枚举的值是**常量的名字**，而不是它们在线上的取值。
docgen 会对无法读懂的指令发出警告。

**字段**上，写在 struct tag 里：

```go
Notes string `json:"notes" apidoc:"internal"`   // 仅内部文档
Beta  string `json:"beta"  apidoc:"public"`     // 仅公开文档
Cost  int    `json:"cost"  apidoc:"hidden"`     // 完全不显示（`apidoc:"-"` 同）
```

文档还会去掉名字以它的 `hide_type_prefixes` 中某个前缀开头的类型，除非该类型自己的 `//apidoc:` 另有说明。
没有注解的内容在所有文档里都显示。

被隐藏的类型没有 schema，会引用它的东西也没有：该类型的字段、它的列表或 map 的字段、内嵌它的 struct 都会被略去，
收发它的接口也会被略去（并有警告说明是哪个）。枚举是例外：它会显示，只是值被隐藏。

旧形式 `apidoc:"Staff"` 只在 `legacy_field_tokens` 里列了 `Staff` 的文档中显示该字段。tag 的其他任何值都会让该字段
在所有文档中被隐藏，docgen 会为此警告：手误和把描述写进了错误的 tag，是以这种方式丢字段的两种常见原因。
tag `api.header:"X-Request-Id"` 给 header 类型的字段指定它所用的 HTTP header 名。

## 配置

`docgen.yaml` 是严格的：未知的键（会列出那里有哪些键，以及它多半是哪个键的笔误）、非法的值、写了斜杠的类型键都是指明键名的错误；
无法解码的键会一起报告，非法的值也是。模块里没有声明的类型被配置键点名，以及没有匹配到任何包的模式，是警告。
[petstore 的配置](examples/petstore/docgen.yaml)是完整的例子，下面是参考：

```yaml
version: 1                      # 唯一的版本

api:
  prefix: /api                  # 每条路由的前缀；默认 /api
  skip_methods: [GetChildren]   # 除 Name 外永远不是接口的方法

vendor_extensions: false        # 添加 x-apifox-*、x-enum-varnames、x-enum-comments、
                                # x-display-name、x-primary-property、x-go-interface
keep_empty_tags: false          # 保留没有 @tags 的接口上的空 tag

compat:                         # 仅用于旧工具生成的文档，
  legacy_operation_types: false #   见“此前生成的文档”
  legacy_schema_shapes: false

type_map:                       # 直接指定某个类型的 schema，不去读它
  example.com.shop.types.ID: {type: string, description: id}
  example.com.shop.types.IDs: {type: array, items: {type: string}}
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
  query:                        # 从 URL query 读取的请求类型：没有请求体。参数是手写的，
                                #   对每个收该类型的接口都生效，不会和它的字段核对
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
`after_apis` 的 overlay 在之后应用。overlay 里的 schema 只有 `$ref`、`title`、`description`、`type`、`format`、`required`、
`properties`、`items`、`additionalProperties` 和 `oneOf`；其他任何键（`enum`、`example`、`nullable`、`allOf` 和各种约束）
都会被拒绝，并列出认识的键。要给类型这些值，请在 Go 里描述它，或用 `type_map`。

## 命令行

```
docgen [-C dir] [-config file] [-doc name]... [-check] [-v] [-cpuprofile file] [-memprofile file]
docgen -version
```

| 参数 | |
|---|---|
| `-C dir` | 像在 `dir` 中启动一样运行，`dir` 是要生成文档的模块根目录。 |
| `-config file` | 配置文件，相对于 `dir`；默认 `docgen.yaml`。 |
| `-doc name` | 只生成该名字的文档；可重复。 |
| `-check` | 生成到临时目录，与磁盘上的文件比较，有不同则以 1 退出。不写任何文件。 |
| `-v` | 额外记录有助于跟踪一次运行的信息，例如被注解隐藏的每个类型。 |
| `-cpuprofile file`、`-memprofile file` | 写出 CPU 或内存 profile，用于查找运行为什么慢。 |
| `-version` | 打印版本。 |

警告输出到标准错误；与代码有关的警告会指出位置。警告不会让运行失败：它们说的是源码或配置里有什么东西不起作用，
或者起的作用和看上去不一样，请把它们当作待办清单。`-check` 比较的是文档的文本，
并且把换行符被替换成回车加换行的检出（`core.autocrlf`）视为没有过期。

在 CI 里：

```sh
go tool docgen -check
```

## 此前生成的文档

docgen 是从一个大型服务的文档生成工具里抽出来的，那些文档已经评审并入库多年。最初的工具有几处做得不对，docgen 修正了它们；
而拥有这类文档的项目不希望升级把它们重写一遍，所以每个会改变它们的修正都有一个开关，用来保留旧的读法。新项目不应该设置它们。

```yaml
compat:
  legacy_operation_types: true
  legacy_schema_shapes: true
```

- `legacy_operation_types`：接口的参数和返回值类型按旧方式读取：列表就是它的元素类型（`[]Pet` 是 `Pet`），
  map、接口和实例化的泛型类型是未知类型，所以它们作为返回值时没有 content。
- `legacy_schema_shapes`：一次声明多个名字（`Lat, Lng float64`）只有第一个是字段，空白字段是名为 `_` 的属性；
  带 json 名字的内嵌 struct 被展开；`validate:"required,email"` 不会让字段必填，只有 `validate:"required"` 才会；
  `any` 是 `object`，`interface{}` 是字符串、整数或对象；枚举的值是用该类型和字面量声明的常量，
  所以 `iota` 和表达式给出空值，隐式重复类型的常量缺失；map 的值类型丢失（`additionalProperties: true`）；
  字节切片是字符串数组，byte 是字符串；枚举类型在 `type_map` 里的条目被忽略。

## 已知限制

docgen 还很年轻。以下限制已知，后续版本会改变，请不要依赖它们。

- **其他模块的类型就地展开**，作为内联 schema 而不是组件；其中包含自身的类型第二次出现时只是一个 `object`。
  不要把它们用作接口的请求或响应类型：用你模块里的类型包一层。自定义的 marshaler 不会被读取，
  所以写出来的形式与 Go 形状不同的类型（`net.IP`、`big.Int`、字节数组的 UUID）需要 `type_map` 条目。
- **公开文档是扁平的**：字段所用的类型就地展开而不是引用，以名字作为 title，只有接口收发的类型才是组件。
  很深或菱形的类型图会让它变得很大。从未被填充的 schema 上可能出现标记 `pattern: default`。
- `json:",string"` 被忽略，指针除非写了 `json:"x,nullable"` 否则不可为 null，`json.RawMessage` 是 `string`（它其实可以是任意 JSON）。
- **泛型按声明文档化**：类型参数类型的字段没有 schema，有多个类型参数的泛型类型不受支持。
- **query 参数是在 `request.query` 里手写的**，不是从 struct 读取的，对每个收该类型的接口都生效。
- 类型的注释不是它的描述，内部文档会略去 `$ref` 字段的注释。
- 接口的请求和响应类型必须声明在匹配 `models` 的包里。
- 文档是 OpenAPI 3.0；kin-openapi 不写 3.1。
- 每次运行都会加载并类型检查整个模块（`./...`），模块里任何包无法构建时都会失败，并说明原因。

## 兼容性

1.0 之前，命令行、配置和输出都可能在次版本中变化。变化记录在 [CHANGELOG.md](CHANGELOG.md)，同一版本内的输出不变。
请固定版本，并把文档入库。

唯一可以 import 的 Go 包是没有任何依赖的 [`route`](route)，其余都是 internal。

## 贡献

见 [CONTRIBUTING.md](CONTRIBUTING.md)。简言之：`go test ./...` 必须通过，
`go run ./cmd/docgen -C examples/petstore -check` 必须成功。

## 许可证

[MIT](LICENSE)。docgen 所依赖的第三方代码列在 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
