(ns mdbrain.markdown
  "Markdown rendering with Obsidian syntax support
   使用 commonmark-java 实现，支持 YAML front matter 自动忽略
   采用第一性原理：每个函数只做一件事，可独立测试"
  (:require [clojure.string :as str]
            [mdbrain.object-store :as object-store]
            [mdbrain.db :as db])
  (:import [org.commonmark.parser Parser]
           [org.commonmark.renderer.html HtmlRenderer]
           [org.commonmark.renderer.text TextContentRenderer]
           [org.commonmark.node Heading]
           [org.commonmark.ext.front.matter YamlFrontMatterExtension]
           [org.commonmark.ext.gfm.tables TablesExtension]
           [org.commonmark.ext.task.list.items TaskListItemsExtension]
           [org.commonmark.ext.gfm.strikethrough StrikethroughExtension]))

;;; ============================================================
;;; 1. CommonMark 解析器初始化
;;; ============================================================

(def ^:private parser-extensions
  "CommonMark 解析扩展列表（含 YAML front matter 与 GFM 支持）"
  [(YamlFrontMatterExtension/create)
   (TablesExtension/create)
   (TaskListItemsExtension/create)
   (StrikethroughExtension/create)])

(def ^:private renderer-extensions
  "HTML 渲染扩展列表（排除 YAML front matter）"
  [(TablesExtension/create)
   (TaskListItemsExtension/create)
   (StrikethroughExtension/create)])

(def ^:private parser
  "线程安全的 Markdown 解析器（单例）
   配置 YAML front matter 扩展用于识别和跳过 front matter"
  (-> (Parser/builder)
      (.extensions parser-extensions)
      (.build)))

(def ^:private renderer
  "线程安全的 HTML 渲染器（单例）
   注意：不添加 YAML 扩展，这样 YamlFrontMatterBlock 节点将被忽略
   配置 softbreak 为 <br> 以保留单行换行"
  (-> (HtmlRenderer/builder)
      (.extensions renderer-extensions)
      (.softbreak "<br>\n")
      (.build)))

(def ^:private text-renderer
  "线程安全的纯文本渲染器（单例）
   用于从 AST 节点提取纯文本内容"
  (-> (TextContentRenderer/builder)
      (.build)))

;;; ============================================================
;;; 1. 基础 Markdown 转换
;;; ============================================================

(declare asset-embed?)

(defn md->html
  "将 Markdown 转换为 HTML
   输入: Markdown 字符串
   输出: HTML 字符串
   
   特性:
   - 自动忽略 YAML front matter (--- ... ---)
   - 支持 CommonMark 规范"
  [md-str]
  (when md-str
    (let [trimmed (str/trim md-str)
          document (.parse parser trimmed)
          html (.render renderer document)]
      (str/trim html))))

;;; ============================================================
;;; 2. Obsidian 链接解析
;;; ============================================================

(def ^:private code-block-pattern
  #"(?:```|~~~)[\s\S]*?(?:```|~~~)")

(def ^:private inline-code-pattern
  #"`[^`\n]*`")

(defn- mask-code
  "将代码块与行内代码替换为占位符，避免解析误伤"
  [content]
  (let [segments (atom [])
        replace-segment (fn [match]
                          (let [idx (count @segments)]
                            (swap! segments conj match)
                            (str "MB_CODE_SEGMENT_" idx "_MB")))]
    {:content (-> content
                  (str/replace code-block-pattern replace-segment)
                  (str/replace inline-code-pattern replace-segment))
     :segments @segments}))

(defn- unmask-code
  "将占位符还原为原始代码块/行内代码"
  [content segments]
  (reduce-kv (fn [acc idx segment]
               (str/replace acc (str "MB_CODE_SEGMENT_" idx "_MB") segment))
             content
             (vec segments)))

(defn- normalize-asset-path
  "规范化资源路径，去除引号、URL 编码、查询参数、片段等"
  [path]
  (when (and path (not (str/blank? path)))
    (let [trimmed (-> path
                      str/trim
                      (str/replace #"^['\"]|['\"]$" "")
                      (str/replace #"^<|>$" "")
                      (str/replace #"^\./" "")
                      (str/replace #"\\" "/"))
          decoded (try
                    (java.net.URLDecoder/decode trimmed "UTF-8")
                    (catch Exception _ trimmed))
          cut-at (or (str/index-of decoded "?")
                     (str/index-of decoded "#"))]
      (if cut-at
        (subs decoded 0 cut-at)
        decoded))))

(defn- asset-url
  "根据资产路径生成可访问的 URL"
  [vault-id path]
  (let [normalized (normalize-asset-path path)
        escaped-path (-> (or normalized path "")
                         (str/replace #"^/+" "")
                         (java.net.URLEncoder/encode "UTF-8")
                         (str/replace "%2F" "/")
                         (str/replace "+" "%20"))
        asset (when normalized (db/find-asset vault-id normalized))]
    (if asset
      (or (object-store/public-asset-url vault-id (:object-key asset))
          (str "/storage/" (:object-key asset)))
      (str "/storage/" escaped-path))))

(defn parse-obsidian-link
  "解析单个 Obsidian 链接
   输入: 链接字符串，如 '[[filename]]' 或 '![[image.png]]'
   输出: {:type :link/:embed
          :embed? true/false
          :path \"文件路径\"
          :display \"显示文本\"
          :anchor \"锚点\" (可选)}

   示例:
   [[filename]] -> {:type :link :embed? false :path \"filename\" :display \"filename\" :anchor nil}
   [[file|text]] -> {:type :link :embed? false :path \"file\" :display \"text\" :anchor nil}
   [[file#head]] -> {:type :link :embed? false :path \"file\" :display \"file#head\" :anchor \"head\"}
   ![[image]] -> {:type :embed :embed? true :path \"image\" :display \"image\" :anchor nil}"
  [link-str]
  (when link-str
    (let [;; 检查是否是嵌入 ![[...]]
          embed? (str/starts-with? link-str "!")
          ;; 提取 [[ 和 ]] 之间的内容
          inner (-> link-str
                    (str/replace #"^!?\[\[" "")
                    (str/replace #"\]\]$" ""))
          ;; 分割 | 获取路径和显示文本
          [path-part display-part] (str/split inner #"\|" 2)
          ;; 分割 # 获取文件和锚点
          [path anchor] (str/split path-part #"#" 2)
          ;; 显示文本（优先使用自定义显示文本，否则使用完整路径部分）
          display-text (or display-part path-part)]
      {:type (if embed? :embed :link)
       :embed? embed?
       :path (str/trim path)
       :display (str/trim display-text)
       :anchor (when anchor (str/trim anchor))})))

(defn- split-link-destination
  "拆分 Markdown 链接目标与 title 部分"
  [raw]
  (let [trimmed (str/trim raw)]
    (if (str/starts-with? trimmed "<")
      (let [end-idx (str/index-of trimmed ">")
            dest (if (and end-idx (pos? end-idx))
                   (subs trimmed 1 end-idx)
                   "")
            rest (if end-idx (subs trimmed (inc end-idx)) "")]
        [dest rest])
      (let [[dest rest] (str/split trimmed #"\s+" 2)]
        [dest (when rest (str " " rest))]))))

(defn- rewrite-inline-images
  "重写 Markdown 图片链接中的资源路径"
  [content vault-id]
  (let [pattern #"!\[([^\]]*)\]\(([^)]+)\)"]
    (str/replace content pattern
                 (fn [[_ alt inner]]
                   (let [[dest rest] (split-link-destination inner)
                         normalized (normalize-asset-path dest)]
                     (if (and normalized (asset-embed? normalized))
                       (str "![" alt "](" (asset-url vault-id normalized) (or rest "") ")")
                       (str "![" alt "](" inner ")")))))))

(defn- rewrite-reference-definitions
  "重写 reference-style 图片的定义 URL"
  [content vault-id]
  (let [pattern #"(?m)^\s*\[([^\]]+)\]:\s+(.+)$"]
    (str/replace content pattern
                 (fn [[line label rest]]
                   (let [[dest tail] (split-link-destination rest)
                         normalized (normalize-asset-path dest)]
                     (if (and normalized (asset-embed? normalized))
                       (str "[" label "]: " (asset-url vault-id normalized) (or tail ""))
                       line))))))

(defn- normalize-reference-label
  [label]
  (-> label str/trim str/lower-case (str/replace #"\s+" " ")))

(defn- extract-reference-definitions
  [content]
  (let [pattern #"(?m)^\s*\[([^\]]+)\]:\s+(.+)$"]
    (reduce (fn [acc [_ label rest]]
              (let [[dest _] (split-link-destination rest)
                    normalized (normalize-asset-path dest)]
                (assoc acc (normalize-reference-label label) normalized)))
            {}
            (re-seq pattern content))))

(defn- rewrite-reference-images
  "将 reference-style 图片重写为 inline 形式，便于渲染"
  [content vault-id]
  (let [definitions (extract-reference-definitions content)
        pattern #"!\[([^\]]*)\]\[([^\]]*)\]"]
    (str/replace content pattern
                 (fn [[original alt label]]
                   (let [resolved-label (normalize-reference-label (if (str/blank? label) alt label))
                         destination (get definitions resolved-label)
                         normalized (normalize-asset-path destination)]
                     (if (and normalized (asset-embed? normalized))
                       (str "![" alt "](" (asset-url vault-id normalized) ")")
                       original))))))

(defn- rewrite-html-media-tags
  "重写 HTML img/audio/video/source 标签中的 src"
  [content vault-id]
  (let [pattern #"<(img|audio|video|source)\b[^>]*>"]
    (str/replace content pattern
                 (fn [[tag]]
                   (let [src-match (re-find #"src\s*=\s*(?:\"([^\"]+)\"|'([^']+)'|([^\s>]+))" tag)
                         raw (or (nth src-match 1 nil) (nth src-match 2 nil) (nth src-match 3 nil))
                         normalized (normalize-asset-path raw)]
                     (if (and normalized (asset-embed? normalized))
                       (str/replace tag
                                    #"src\s*=\s*(?:\"([^\"]+)\"|'([^']+)'|([^\s>]+))"
                                    (str "src=\"" (asset-url vault-id normalized) "\""))
                       tag))))))

(defn- rewrite-asset-links
  "重写 Markdown 内容中的资源链接（图片/HTML）"
  [content vault-id]
  (let [{:keys [content segments]} (mask-code content)
        updated (-> content
                    (rewrite-inline-images vault-id)
                    (rewrite-reference-images vault-id)
                    (rewrite-reference-definitions vault-id)
                    (rewrite-html-media-tags vault-id))]
    (unmask-code updated segments)))

;;; ============================================================
;;; 3. 数学公式处理
;;; ============================================================

(defn extract-math
  "从内容中提取数学公式，替换为占位符
   输入: 包含 LaTeX 的字符串
   输出: {:content \"替换后的内容\"
          :formulas [{:type :inline/:block :formula \"公式内容\"}...]}

   支持:
   - 行内公式: $x^2$
   - 块级公式: $$E = mc^2$$"
  [content]
  (let [formulas (atom [])
        ;; 1. 先提取块级公式 $$...$$
        content-1 (str/replace content
                               #"\$\$([^\$]+?)\$\$"
                               (fn [[_ formula]]
                                 (let [idx (count @formulas)]
                                   (swap! formulas conj {:type :block :formula (str/trim formula)})
                                   (str "MATHBLOCK" idx "MATHBLOCK"))))
        ;; 2. 再提取行内公式 $...$
        content-2 (str/replace content-1
                               #"(?<!\$)\$(?!\$)([^\$\n]+?)\$(?!\$)"
                               (fn [[_ formula]]
                                 (let [idx (count @formulas)]
                                   (swap! formulas conj {:type :inline :formula (str/trim formula)})
                                   (str "MATHINLINE" idx "MATHINLINE"))))]
    {:content content-2
     :formulas @formulas}))

(defn restore-math
  "将占位符还原为 KaTeX 标记
   输入: HTML 字符串和公式列表
   输出: 替换后的 HTML

   占位符格式: MATHINLINE0MATHINLINE, MATHBLOCK0MATHBLOCK, ...
   输出格式:
   - 行内: <span class=\"math-inline\">formula</span>
   - 块级: <div class=\"math-block\">formula</div>"
  [html formulas]
  (reduce-kv
   (fn [result idx {:keys [type formula]}]
     (let [placeholder (if (= type :inline)
                         (str "MATHINLINE" idx "MATHINLINE")
                         (str "MATHBLOCK" idx "MATHBLOCK"))
           replacement (if (= type :inline)
                         (format "<span class=\"math-inline\">%s</span>" formula)
                         (format "<div class=\"math-block\">%s</div>" formula))]
       (str/replace result placeholder replacement)))
   html
   (vec formulas)))

;;; ============================================================
;;; 4. 提取元数据
;;; ============================================================

(defn- strip-yaml-front-matter
  "移除 YAML front matter（--- ... ---）
   输入: Markdown 字符串
   输出: 移除 front matter 后的字符串"
  [content]
  (if (str/starts-with? content "---")
    (let [lines (str/split-lines content)
          ;; 找到第二个 --- 的位置
          rest-lines (rest lines)
          end-idx (loop [idx 0
                         remaining rest-lines]
                    (if (empty? remaining)
                      nil
                      (if (str/starts-with? (first remaining) "---")
                        idx
                        (recur (inc idx) (rest remaining)))))]
      (if end-idx
        (str/join "\n" (drop (+ 2 end-idx) lines))
        content))
    content))

(defn- find-first-h1
  "遍历 AST 找到第一个 H1 节点
   输入: CommonMark Document 节点
   输出: Heading 节点或 nil"
  [document]
  (loop [node (.getFirstChild document)]
    (when node
      (if (and (instance? Heading node)
               (= 1 (.getLevel node)))
        node
        (recur (.getNext node))))))

(defn- get-heading-text
  "从 Heading 节点提取纯文本内容
   输入: Heading 节点
   输出: 文本字符串"
  [heading]
  (str/trim (.render text-renderer heading)))

(defn extract-title
  "从 Markdown 内容中提取 H1 标题（使用 CommonMark AST）
   输入: Markdown 字符串
   输出: H1 标题字符串或 nil
   
   特性:
   - 只提取 H1（# 开头），H2-H6 不提取
   - 自动跳过 YAML front matter
   - 不会误提取代码块内的 # 注释
   - 多个 H1 时返回第一个"
  [content]
  (when (and content (not (str/blank? content)))
    (let [document (.parse parser (str/trim content))
          h1-node (find-first-h1 document)]
      (when h1-node
        (get-heading-text h1-node)))))

(defn- image-line?
  "检查一行是否是图片行
   支持:
   - 标准 Markdown: ![alt](url) 或 ![alt][ref]
   - Obsidian 嵌入: ![[image.png]]"
  [line]
  (let [trimmed (str/trim line)]
    (or (re-matches #"!\[.*\]\(.*\)" trimmed)      ; ![alt](url)
        (re-matches #"!\[.*\]\[.*\]" trimmed)      ; ![alt][ref]
        (re-matches #"!\[\[.*\]\]" trimmed))))     ; ![[image]]

(defn extract-description
  "从 Markdown 内容中提取描述（第一段非标题、非图片文本）
   输入: Markdown 字符串, 可选的最大长度（默认 100）
   输出: 描述字符串或 nil
   
   注意: 会跳过 YAML front matter、标题行和图片行"
  ([content] (extract-description content 100))
  ([content max-length]
   (when content
     (let [body (strip-yaml-front-matter content)
           lines (str/split-lines body)
           first-para (->> lines
                           (drop-while #(or (str/starts-with? % "#")
                                            (str/blank? %)))
                           (filter #(and (not (str/blank? %))
                                         (not (image-line? %))))
                           first)]
       (when first-para
         (subs first-para 0 (min max-length (count first-para))))))))

;;; ============================================================
;;; 5. Obsidian 链接替换
;;; ============================================================

(defn- build-link-index
  "从链接列表构建原始链接到链接数据的索引
   输入: 链接列表 [{:original \"[[...]]\" :target-client-id \"...\" ...}]
   输出: {\"[[Note A]]\" {:target-client-id \"...\" ...}}"
  [links]
  (into {}
        (map (fn [link]
               [(:original link) link])
             links)))

(defn- find-link-by-original
  "通过原始链接字符串查找链接数据
   输入: 原始链接字符串（如 \"[[Note A]]\"）和链接索引
   输出: 链接 map 或 nil"
  [original link-index]
  (get link-index original))

(defn- render-internal-link
  "渲染内部链接为 HTML
   输入: 目标 client-id、显示文本、可选锚点
   输出: HTML 字符串"
  [target-client-id display-text anchor]
  (let [href (if anchor
               (format "/%s#%s" target-client-id anchor)
               (format "/%s" target-client-id))]
    (format "<a href=\"%s\" class=\"internal-link\" data-note-id=\"%s\">%s</a>"
            href
            target-client-id
            (str/escape display-text {\< "&lt;" \> "&gt;" \" "&quot;"}))))

(defn- render-broken-link
  "渲染不存在的链接为 HTML
   输入: 路径、显示文本
   输出: HTML 字符串"
  [path display-text]
  (format "<span class=\"internal-link broken\" title=\"Note not found: %s\">%s</span>"
          (str/escape path {\< "&lt;" \> "&gt;" \" "&quot;"})
          (str/escape display-text {\< "&lt;" \> "&gt;" \" "&quot;"})))

(defn- render-image-embed
  "渲染图片嵌入为 HTML
   输入: 目标 client-id、显示文本
   输出: HTML 字符串"
  [target-client-id display-text]
  (format "<img src=\"/notes/%s/content\" alt=\"%s\" class=\"obsidian-embed\">"
          target-client-id
          (str/escape display-text {\< "&lt;" \> "&gt;" \" "&quot;"})))

(defn- asset-embed?
  "检查路径是否为资源文件（非 .md 文件）
   输入: 文件路径
   输出: true/false"
  [path]
  (when path
    (let [lower-path (str/lower-case path)]
      (and (not (str/ends-with? lower-path ".md"))
           (or (str/ends-with? lower-path ".png")
               (str/ends-with? lower-path ".jpg")
               (str/ends-with? lower-path ".jpeg")
               (str/ends-with? lower-path ".gif")
               (str/ends-with? lower-path ".webp")
               (str/ends-with? lower-path ".svg")
               (str/ends-with? lower-path ".bmp")
               (str/ends-with? lower-path ".ico")
               (str/ends-with? lower-path ".pdf")
               (str/ends-with? lower-path ".mp3")
               (str/ends-with? lower-path ".mp4")
               (str/ends-with? lower-path ".webm")
               (str/ends-with? lower-path ".ogg")
               (str/ends-with? lower-path ".wav"))))))

(defn- render-asset-embed
  "渲染资源嵌入为 HTML
   输入: vault-id、资源路径、显示文本
   输出: HTML 字符串
   
   资源查找策略 (Obsidian 风格):
   1. 先精确匹配 path
   2. 找不到则按文件名匹配 (支持短链接如 ![[image.png]])
   
   URL 生成:
   - 如果配置了 S3，使用 S3 直接访问 (public URL)
   - 否则使用相对路径 /storage/..."
  [vault-id path display-text]
  (let [normalized (normalize-asset-path path)
        escaped-display (str/escape display-text {\< "&lt;" \> "&gt;" \" "&quot;"})
        asset-url (asset-url vault-id normalized)]
    (cond
      ;; Image files
      (re-matches #".*\.(png|jpg|jpeg|gif|webp|svg|bmp|ico)$" (str/lower-case (or normalized "")))
      (format "<img src=\"%s\" alt=\"%s\" class=\"asset-embed\">"
              asset-url escaped-display)
      
      ;; PDF files
      (str/ends-with? (str/lower-case (or normalized "")) ".pdf")
      (format "<a href=\"%s\" class=\"asset-link pdf-link\">%s</a>"
              asset-url escaped-display)
      
      ;; Audio files
      (re-matches #".*\.(mp3|ogg|wav)$" (str/lower-case (or normalized "")))
      (format "<audio src=\"%s\" controls class=\"asset-embed\">%s</audio>"
              asset-url escaped-display)
      
      ;; Video files
      (re-matches #".*\.(mp4|webm)$" (str/lower-case (or normalized "")))
      (format "<video src=\"%s\" controls class=\"asset-embed\">%s</video>"
              asset-url escaped-display)
      
      ;; Fallback to link
      :else
      (format "<a href=\"%s\" class=\"asset-link\">%s</a>"
              asset-url escaped-display))))

(defn- render-asset-link
  "渲染资源链接为 HTML"
  [vault-id path display-text]
  (let [normalized (normalize-asset-path path)
        escaped-display (str/escape display-text {\< "&lt;" \> "&gt;" \" "&quot;"})
        asset-url (asset-url vault-id normalized)]
    (format "<a href=\"%s\" class=\"asset-link\">%s</a>"
            asset-url escaped-display)))

(defn replace-obsidian-links
  "替换文本中的所有 Obsidian 链接为 HTML 链接
   输入: 内容字符串、vault-id、链接列表 [{:original \"[[...]]\" :target-client-id \"...\" :display-text \"...\" :link-type \"link\"/\"embed\"}]
   输出: 替换后的字符串

   处理:
   - [[链接]] -> 内部链接
   - [[链接|文本]] -> 带自定义文本的链接
   - [[链接#锚点]] -> 带锚点的链接
   - ![[图片.png]] -> 资源嵌入 (使用 S3 public URL)
   - ![[note.md]] -> 笔记嵌入
   
    注意: links 参数来自 note_links 表，只包含已解析的有效链接
         资源文件（图片、音视频等）不在 note_links 表中，直接渲染为资源链接"
  [content vault-id links]
  (let [{:keys [content segments]} (mask-code content)
        link-index (build-link-index links)
        pattern #"(!?)\[\[([^\]]+)\]\]"
        replaced (str/replace content pattern
                              (fn [[full-match is-embed _link-text]]
                                (let [original full-match
                                      parsed (parse-obsidian-link original)
                                      path (:path parsed)
                                      display (:display parsed)
                                      normalized (normalize-asset-path path)]
                                  (cond
                                    ;; 1. 嵌入且是资源文件（图片、音视频等）
                                    (and (= is-embed "!") (asset-embed? normalized))
                                    (render-asset-embed vault-id normalized display)

                                    ;; 2. 非嵌入但链接到资源文件 -> 渲染为资源链接
                                    (and (not= is-embed "!") (asset-embed? normalized))
                                    (render-asset-link vault-id normalized display)

                                    ;; 3. 链接存在于预存储的列表中（note_links 表）
                                    (find-link-by-original original link-index)
                                    (let [link-data (get link-index original)
                                          target-client-id (:target-client-id link-data)
                                          display-text (:display-text link-data)
                                          link-type (:link-type link-data)]
                                      (if (= link-type "embed")
                                        (render-image-embed target-client-id display-text)
                                        (render-internal-link target-client-id display-text (:anchor parsed))))

                                    ;; 4. 链接不存在（broken link）
                                    :else
                                    (render-broken-link path display)))))]
    (unmask-code replaced segments)))

;;; ============================================================
;;; 6. 完整渲染流程
;;; ============================================================

(defn render-markdown
  "完整的 Markdown 渲染流程
   输入: Markdown 内容、vault-id、链接列表 [{:original \"[[...]]\" :target-client-id \"...\" ...}]
   输出: HTML 字符串

   处理流程:
   1. 提取并替换数学公式为占位符
   2. 替换 Obsidian 链接为 HTML 链接
   3. Markdown -> HTML 转换
   4. 还原数学公式为 KaTeX 标记"
  [content vault-id links]
  (when content
    (let [;; 1. 提取数学公式
          {:keys [content formulas]} (extract-math content)
          ;; 2. 替换 Obsidian 链接
          content-with-assets (rewrite-asset-links content vault-id)
          content-with-links (replace-obsidian-links content-with-assets vault-id links)
          ;; 3. Markdown -> HTML
          html (md->html content-with-links)
          ;; 4. 还原数学公式
          final-html (restore-math html formulas)]
      final-html)))
