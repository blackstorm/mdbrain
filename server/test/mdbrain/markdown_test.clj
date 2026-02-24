(ns mdbrain.markdown-test
  (:require [clojure.test :refer :all]
            [clojure.string :as str]
            [mdbrain.db :as db]
            [mdbrain.markdown :as md]))

(defn with-stub-asset-lookups [f]
  (with-redefs [db/find-asset (fn [_ _] nil)]
    (f)))

(use-fixtures :each with-stub-asset-lookups)

;;; 测试 1: 基础 Markdown 转换
(deftest test-basic-markdown-conversion
  (testing "将简单的 markdown 转换为 HTML"
    (let [html (md/md->html "Hello world")]
      (is (str/includes? html "<p>"))
      (is (str/includes? html "Hello world"))))

  (testing "标题转换"
    (let [html (md/md->html "# Title")]
      (is (str/includes? html "<h1>"))
      (is (str/includes? html "Title"))))

  (testing "粗体和斜体"
    (let [html (md/md->html "**bold** and *italic*")]
      (is (str/includes? html "<strong>bold</strong>"))
      (is (str/includes? html "<em>italic</em>"))))

  (testing "代码块"
    (let [html (md/md->html "```python\nprint('hello')\n```")]
      (is (str/includes? html "<code"))
      (is (str/includes? html "print"))))

  (testing "nil 输入返回 nil"
    (is (nil? (md/md->html nil)))))

;;; 测试 1.5: YAML Front Matter 处理
(deftest test-yaml-front-matter
  (testing "忽略 YAML front matter，只渲染内容"
    (let [content "---\ntitle: Hello\ntags: [clojure]\n---\n\n# Heading\n\nBody text."
          html (md/md->html content)]
      ;; 不应包含 YAML 标记
      (is (not (str/includes? html "---")))
      (is (not (str/includes? html "title:")))
      (is (not (str/includes? html "tags:")))
      ;; 应包含正文内容
      (is (str/includes? html "<h1>"))
      (is (str/includes? html "Heading"))
      (is (str/includes? html "Body text"))))

  (testing "没有 front matter 的内容正常渲染"
    (let [html (md/md->html "# Just content\n\nNo front matter here.")]
      (is (str/includes? html "<h1>"))
      (is (str/includes? html "Just content"))))

  (testing "只有 front matter 的内容渲染为空"
    (let [html (md/md->html "---\ntitle: Only metadata\n---")]
      ;; 应该返回空或只有空白
      (is (or (nil? html)
              (str/blank? (str/trim html))
              (not (str/includes? html "title"))))))

  (testing "复杂 YAML front matter"
    (let [content "---\ntitle: Complex\nauthor: Test\ndate: 2024-01-01\ntags:\n  - tag1\n  - tag2\n---\n\nContent here."
          html (md/md->html content)]
      (is (not (str/includes? html "Complex")))
      (is (not (str/includes? html "author:")))
      (is (str/includes? html "Content here"))))

  (testing "thematic break (---) 在正文中不被误解析"
    (let [content "# Title\n\nSome text\n\n---\n\nMore text"
          html (md/md->html content)]
      ;; --- 在正文中应该渲染为 <hr>
      (is (str/includes? html "<hr"))
      (is (str/includes? html "Some text"))
      (is (str/includes? html "More text")))))

;;; 测试 1.6: GFM 扩展（tables / task list / strikethrough）
(deftest test-gfm-extensions
  (testing "GFM table 渲染为 HTML table"
    (let [content "| A | B |\n| --- | --- |\n| 1 | 2 |"
          html (md/md->html content)]
      (is (str/includes? html "<table>"))
      (is (str/includes? html "<th>"))
      (is (str/includes? html "<td>"))))

  (testing "GFM task list 渲染为 checkbox"
    (let [content "- [x] Done\n- [ ] Todo"
          html (md/md->html content)]
      (is (str/includes? html "type=\"checkbox\""))
      (is (str/includes? html "checked"))
      (is (str/includes? html "disabled"))))

  (testing "GFM strikethrough 渲染为 <del>"
    (let [html (md/md->html "~~gone~~")]
      (is (str/includes? html "<del>gone</del>")))))

;;; 测试 2: 解析单个 Obsidian 链接
(deftest test-parse-obsidian-link
  (testing "解析简单链接 [[filename]]"
    (is (= {:type :link
            :embed? false
            :path "filename"
            :display "filename"
            :anchor nil}
           (md/parse-obsidian-link "[[filename]]"))))

  (testing "解析带显示文本的链接 [[filename|display text]]"
    (is (= {:type :link
            :embed? false
            :path "filename"
            :display "display text"
            :anchor nil}
           (md/parse-obsidian-link "[[filename|display text]]"))))

  (testing "解析带锚点的链接 [[filename#heading]]"
    (is (= {:type :link
            :embed? false
            :path "filename"
            :display "filename#heading"
            :anchor "heading"}
           (md/parse-obsidian-link "[[filename#heading]]"))))

  (testing "解析图片嵌入 ![[image.png]]"
    (is (= {:type :embed
            :embed? true
            :path "image.png"
            :display "image.png"
            :anchor nil}
           (md/parse-obsidian-link "![[image.png]]")))))

;;; 测试 3: 提取数学公式
(deftest test-extract-math
  (testing "提取行内公式 $x^2$"
    (let [result (md/extract-math "The formula $x^2$ is simple")]
      (is (= "The formula MATHINLINE0MATHINLINE is simple" (:content result)))
      (is (= [{:type :inline :formula "x^2"}] (:formulas result)))))

  (testing "提取块级公式 $$...$$"
    (let [result (md/extract-math "$$E = mc^2$$")]
      (is (= "MATHBLOCK0MATHBLOCK" (:content result)))
      (is (= [{:type :block :formula "E = mc^2"}] (:formulas result)))))

  (testing "提取多个公式"
    (let [result (md/extract-math "Inline $a + b$ and block $$c = d$$")]
      ;; 注意：先提取块级再提取行内，所以顺序是反的
      (is (= "Inline MATHINLINE1MATHINLINE and block MATHBLOCK0MATHBLOCK" (:content result)))
      (is (= [{:type :block :formula "c = d"}
              {:type :inline :formula "a + b"}]
             (:formulas result))))))

;;; 测试 4: 还原数学公式
(deftest test-restore-math
  (testing "还原行内公式"
    (is (= "<p>The formula <span class=\"math-inline\">x^2</span> is simple</p>"
           (md/restore-math "<p>The formula MATHINLINE0MATHINLINE is simple</p>"
                            [{:type :inline :formula "x^2"}]))))

  (testing "还原块级公式"
    (is (= "<div class=\"math-block\">E = mc^2</div>"
           (md/restore-math "MATHBLOCK0MATHBLOCK"
                            [{:type :block :formula "E = mc^2"}])))))

;;; 测试 5: 提取标题（使用 CommonMark AST，只提取 H1）
(deftest test-extract-title
  (testing "从内容中提取第一个 H1 标题"
    (is (= "My Title"
           (md/extract-title "# My Title\n\nSome content"))))

  (testing "无标题时返回 nil"
    (is (nil? (md/extract-title "Just content without title"))))

  (testing "H2 不被提取（只提取 H1）"
    (is (nil? (md/extract-title "## Subtitle\n\nContent"))))

  (testing "跳过 YAML front matter 提取标题"
    (is (= "Real Title"
           (md/extract-title "---\ntitle: Fake Title\n---\n# Real Title\n\nContent"))))

  (testing "代码块内的 # 不被误提取"
    (is (nil? (md/extract-title "```python\n# This is a comment\n```\n\nNo real title"))))

  (testing "有 H1 时忽略代码块内的注释"
    (is (= "Real Title"
           (md/extract-title "# Real Title\n\n```python\n# comment\n```"))))

  (testing "多个 H1 只取第一个"
    (is (= "First"
           (md/extract-title "# First\n\n# Second"))))

  (testing "H1 在 H2 之后也能正确提取"
    (is (= "Main Title"
           (md/extract-title "## Intro\n\n# Main Title\n\nContent"))))

  (testing "空内容返回 nil"
    (is (nil? (md/extract-title nil))))

  (testing "只有空白返回 nil"
    (is (nil? (md/extract-title "   \n\n  ")))))

;;; 测试 5.5: 提取描述
(deftest test-extract-description
  (testing "提取第一段非标题文本"
    (is (= "This is the description."
           (md/extract-description "# Title\n\nThis is the description.\n\nMore content."))))

  (testing "跳过 YAML front matter"
    (is (= "This is content."
           (md/extract-description "---\ntitle: Test\ntags: [a, b]\n---\n# Title\n\nThis is content."))))

  (testing "跳过多个标题行"
    (is (= "First paragraph."
           (md/extract-description "# H1\n## H2\n\nFirst paragraph."))))

  (testing "限制描述长度"
    (is (= "Short"
           (md/extract-description "# Title\n\nShort description here." 5))))

  (testing "空内容返回 nil"
    (is (nil? (md/extract-description nil))))

  (testing "只有标题没有内容返回 nil"
    (is (nil? (md/extract-description "# Title Only"))))

  (testing "只有 YAML front matter 返回 nil"
    (is (nil? (md/extract-description "---\ntitle: Test\n---"))))

  (testing "跳过 Obsidian 图片嵌入"
    (is (= "This is the actual description."
           (md/extract-description "# Title\n\n![[image.png]]\n\nThis is the actual description."))))

  (testing "跳过标准 Markdown 图片"
    (is (= "This is the description."
           (md/extract-description "# Title\n\n![alt text](https://example.com/img.png)\n\nThis is the description."))))

  (testing "跳过多个连续图片"
    (is (= "Finally some text."
           (md/extract-description "# Title\n\n![[img1.png]]\n![alt](url.jpg)\n![[img2.gif]]\n\nFinally some text."))))

  (testing "只有图片没有文本返回 nil"
    (is (nil? (md/extract-description "# Title\n\n![[only-image.png]]"))))

  (testing "跳过图片引用格式"
    (is (= "Description here."
           (md/extract-description "# Title\n\n![alt][ref]\n\nDescription here.\n\n[ref]: http://example.com/img.png")))))

;;; 测试 6: 替换 Obsidian 链接（使用预存储的 links）
(deftest test-replace-obsidian-links
  (let [;; 模拟从 document_links 表查询的链接数据
        ;; 格式: {:original "[[...]]" :target-client-id "..." :display-text "..."}
        vault-id "test-vault-id"
        links [{:original "[[Note A]]"
                :target-client-id "client-1"
                :target-path "Note A"
                :display-text "Note A"
                :link-type "link"}
               {:original "[[Note B|my note]]"
                :target-client-id "client-2"
                :target-path "Note B"
                :display-text "my note"
                :link-type "link"}
               ;; Note: .md embeds still use client-id
               {:original "![[embedded-note.md]]"
                :target-client-id "client-3"
                :target-path "embedded-note.md"
                :display-text "embedded-note.md"
                :link-type "embed"}]]

    (testing "替换简单链接 - 使用 link 数据"
      (is (str/includes?
           (md/replace-obsidian-links "Check [[Note A]]" vault-id links)
           "href=\"/client-1\"")))

    (testing "替换带显示文本的链接"
      (let [result (md/replace-obsidian-links "See [[Note B|my note]]" vault-id links)]
        (is (str/includes? result "href=\"/client-2\""))
        (is (str/includes? result ">my note</a>"))))

    (testing "链接不存在（不在 links 列表中）时显示为 broken"
      (let [result (md/replace-obsidian-links "[[Non Existent]]" vault-id links)]
        (is (str/includes? result "broken"))
        (is (str/includes? result "Non Existent"))))

    (testing "图片嵌入 - 资源文件直接使用 /storage/ 路径"
      (let [result (md/replace-obsidian-links "![[image.png]]" vault-id [])]
        (is (str/includes? result "<img"))
        (is (str/includes? result "/storage/image.png"))
        (is (str/includes? result "asset-embed"))))

    (testing "非嵌入资源链接 [[image.png]] 渲染为资源链接"
      (let [result (md/replace-obsidian-links "[[image.png]]" vault-id [])]
        (is (str/includes? result "<a"))
        (is (str/includes? result "/storage/image.png"))
        (is (str/includes? result "asset-link"))))
    
    (testing "嵌入 .md 文件 - 使用 client-id"
      (let [result (md/replace-obsidian-links "![[embedded-note.md]]" vault-id links)]
        (is (str/includes? result "<img"))
        (is (str/includes? result "/client-3"))))
    
    (testing "资源嵌入 - 各种格式"
      ;; PNG
      (let [result (md/replace-obsidian-links "![[assets/photo.png]]" vault-id [])]
        (is (str/includes? result "<img"))
        (is (str/includes? result "/storage/assets/photo.png")))
      ;; PDF
      (let [result (md/replace-obsidian-links "![[docs/report.pdf]]" vault-id [])]
        (is (str/includes? result "<a"))
        (is (str/includes? result "/storage/docs/report.pdf"))
        (is (str/includes? result "pdf-link")))
      ;; MP4
      (let [result (md/replace-obsidian-links "![[video.mp4]]" vault-id [])]
        (is (str/includes? result "<video"))
        (is (str/includes? result "/storage/video.mp4")))
      ;; MP3
      (let [result (md/replace-obsidian-links "![[audio.mp3]]" vault-id [])]
        (is (str/includes? result "<audio"))
        (is (str/includes? result "/storage/audio.mp3"))))

    (testing "代码块与行内代码中的 Obsidian 链接不被替换"
      (let [content (str "Inline `[[Note A]]` should stay.\n\n"
                         "```md\n[[Note A]]\n```\n\n"
                         "Outside [[Note A]]")
            result (md/replace-obsidian-links content vault-id links)]
        (is (str/includes? result "`[[Note A]]`"))
        (is (str/includes? result "```md\n[[Note A]]\n```"))
        (is (str/includes? result "href=\"/client-1\"")))))
  )

;;; 测试 7: 完整渲染流程
(deftest test-render-markdown
  (let [;; 模拟预存储的链接数据
        vault-id "test-vault-id"
        links [{:original "[[Other Note]]"
                :target-client-id "client-1"
                :target-path "Other Note"
                :display-text "Other Note"
                :link-type "link"}]
        content "# Test Note\n\nThis is a [[Other Note]] with $x^2$ formula.\n\n$$E = mc^2$$"]

    (testing "完整渲染包含所有功能"
      (let [result (md/render-markdown content vault-id links)]
        ;; 检查 HTML 标题
        (is (str/includes? result "<h1"))
        (is (str/includes? result "Test Note"))
        ;; 检查 Obsidian 链接被替换 - 使用 client-id
        (is (str/includes? result "href=\"/client-1\""))
        ;; 检查数学公式被标记
        (is (str/includes? result "math-inline"))
        (is (str/includes? result "math-block"))))))

;;; 测试 8: 资源链接渲染扩展
(deftest test-render-asset-links
  (let [vault-id "test-vault-id"
        links []
        content (str "Inline image: ![Alt](assets/inline.png)\n\n"
                     "Angle image: ![Alt](<assets/space image.png> \"Title\")\n\n"
                     "Reference image: ![Alt][ref]\n\n"
                     "[ref]: assets/ref.png \"Title\"\n\n"
                     "<img src=\"assets/html.png\" />\n\n"
                     "Fragment: ![Alt](assets/frag.png#section)\n\n"
                     "Query: ![Alt](assets/query.png?raw=1)\n\n"
                     "Inline code `![Alt](assets/code.png)`\n\n"
                     "```md\n![Alt](assets/fenced.png)\n```\n")]

    (testing "Markdown 图片、引用式图片、HTML 标签能正确重写路径"
      (let [result (md/render-markdown content vault-id links)]
        (is (str/includes? result "src=\"/storage/assets/inline.png\""))
        (is (str/includes? result "src=\"/storage/assets/space%20image.png\""))
        (is (str/includes? result "src=\"/storage/assets/ref.png\""))
        (is (str/includes? result "src=\"/storage/assets/html.png\""))
        (is (str/includes? result "src=\"/storage/assets/frag.png\""))
        (is (str/includes? result "src=\"/storage/assets/query.png\""))))

    (testing "代码块与行内代码中的 Markdown 图片不被重写"
      (let [result (md/render-markdown content vault-id links)]
        (is (str/includes? result "![Alt](assets/code.png)"))
        (is (str/includes? result "![Alt](assets/fenced.png)"))))))
