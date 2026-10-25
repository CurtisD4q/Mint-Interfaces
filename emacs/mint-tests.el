;;; mint-tests.el --- Tests for mint.el -*- lexical-binding: t; -*-

;; Run from the repository root (needs Go):
;;   emacs -Q --batch -L emacs -l emacs/mint-tests.el -f ert-run-tests-batch-and-exit

;;; Code:

(require 'ert)
(require 'mint)

(defconst mint-tests-root
  (expand-file-name ".." (file-name-directory (or load-file-name buffer-file-name))))

(defvar mint-tests--bin nil "Built demo binary, shared by the tests.")

(defun mint-tests--demo ()
  "Build examples/demo once into a temporary folder and return its path."
  (or mint-tests--bin
      (let ((out (expand-file-name "demo" (make-temp-file "mint-tests" t))))
        (setq mint-tests--bin (mint-build (expand-file-name "examples/demo" mint-tests-root) out)))))

(ert-deftest mint-info-reads-a-mint-app ()
  (let ((info (mint-app-info (mint-tests--demo))))
    (should (equal (alist-get 'crumb info) "Demo"))
    (should (member "Widgets" (mapcar (lambda (i) (alist-get 'label i)) (alist-get 'items info))))))

(ert-deftest mint-info-gives-up-on-other-programs ()
  ;; A program that ignores --mint-info and never exits must time out, not
  ;; hang Emacs.
  (let* ((script (make-temp-file "mint-silent" nil ".sh" "#!/bin/sh\nsleep 30\n"))
         (mint-info-timeout 1)
         (start (float-time)))
    (set-file-modes script #o755)
    (should (null (mint-app-info script)))
    (should (< (- (float-time) start) 3))
    (should (>= (- (float-time) start) 0.9))))

(ert-deftest mint-info-of-missing-program-is-nil ()
  (should (null (mint-app-info "/definitely/not/a/program"))))

(ert-deftest mint-build-only-rebuilds-when-stale ()
  (let* ((src (make-temp-file "mint-src" t))
         (bin (expand-file-name "app" src)))
    (copy-directory (expand-file-name "templates/app" mint-tests-root) (file-name-as-directory src) nil nil t)
    (with-temp-file (expand-file-name "go.mod" src)
      (insert (format "module example.com/app\n\ngo 1.21\n\nrequire github.com/CurtisD4q/Mint-Interfaces v0.0.0\n\nreplace github.com/CurtisD4q/Mint-Interfaces => %s\n"
                      mint-tests-root)))
    (mint-build src bin)
    (should (file-executable-p bin))
    (should-not (mint--stale-p bin src))
    (let ((before (file-attribute-modification-time (file-attributes bin))))
      (mint-build src bin)                    ; nothing changed: no rebuild
      (should (equal before (file-attribute-modification-time (file-attributes bin))))
      (sleep-for 1.1)
      (set-file-times (expand-file-name "main.go" src))
      (should (mint--stale-p bin src))
      (mint-build src bin)                    ; source changed: rebuilt
      (should-not (equal before (file-attribute-modification-time (file-attributes bin)))))))

(ert-deftest mint-finds-apps-in-folders-and-skips-others ()
  (let* ((dir (make-temp-file "mint-apps" t))
         (other (expand-file-name "not-mint" dir))
         (mint--apps nil)
         (mint-info-timeout 1)
         (mint-app-directories (list dir)))
    (copy-file (mint-tests--demo) (expand-file-name "demo" dir))
    (with-temp-file other (insert "#!/bin/sh\necho hello\n"))
    (set-file-modes other #o755)
    (let ((names (mapcar #'car (mint--candidates))))
      (should (member "Demo" names))
      (should-not (member "not-mint" names)))))

(ert-deftest mint-define-app-registers-and-defines ()
  (let ((mint--apps nil))
    (eval '(mint-define-app mint-tests-demo-cmd "Demo app" "/some/where") t)
    (should (fboundp 'mint-tests-demo-cmd))
    (should (equal (alist-get "Demo app" mint--apps nil nil #'equal) "/some/where"))))

(ert-deftest mint-environment-follows-settings ()
  (let ((mint-key-profile 'vim) (mint-match-theme nil))
    (should (equal (mint--environment) '("MINT_KEYS=vim"))))
  (let ((mint-key-profile nil) (mint-match-theme nil))
    (should (null (mint--environment)))))

(ert-deftest mint-hex-converts-colours ()
  (should (equal (mint--hex "#ff8000") "#ff8000"))
  (should (equal (mint--hex "#FF8000") "#ff8000"))
  (should (null (mint--hex "unspecified-fg")))
  (should (null (mint--hex nil))))

(ert-deftest mint-term-filter-holds-back-split-characters ()
  ;; "━" is three bytes; send it in two pieces right after an escape code.
  (let* ((buf (generate-new-buffer " *mint-term-test*"))
         (proc (start-process "mint-test" buf "sleep" "30")))
    (unwind-protect
        (with-current-buffer buf
          (term-mode)
          (setq term-width 20 term-height 3)
          (let ((bytes (encode-coding-string "\e[1;1H\e[38;5;141m━━x" 'utf-8)))
            (mint--term-filter proc (substring bytes 0 18))
            (mint--term-filter proc (substring bytes 18)))
          (should (string-match-p "━━x" (buffer-string)))
          (should-not (string-match-p "[\x3fff80-\x3fffff]" (buffer-string))))
      (delete-process proc)
      (kill-buffer buf))))

(provide 'mint-tests)
;;; mint-tests.el ends here
