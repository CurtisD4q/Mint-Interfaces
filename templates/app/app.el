;;; app.el --- M-x mytool: open this Mint app from Emacs -*- lexical-binding: t; -*-

;; Copy this file with the starter, rename it to <tool>-mint.el, and replace
;; "mytool" below with the tool's name.  Then load it from your init file:
;;
;;   (add-to-list 'load-path "/path/to/Mint-Interfaces/emacs")   ; for mint.el
;;   (load "/path/to/mytool-ui/mytool-mint.el")
;;
;; It defines `M-x mytool' (with C-u it asks which screen to open) and
;; lists the app in `M-x mint'.  The app is built from this folder the
;; first time, and again whenever its Go files change.

;;; Code:

(defconst mytool-mint-dir
  (file-name-directory (or load-file-name buffer-file-name default-directory))
  "Folder holding this app's Go source.")

;; Inside the Mint-Interfaces repository, find mint.el without load-path.
(unless (featurep 'mint)
  (let ((in-repo (expand-file-name "../../emacs/mint.el" mytool-mint-dir)))
    (if (file-readable-p in-repo)
        (load in-repo nil t)
      (require 'mint))))

(mint-define-app mytool "mytool" mytool-mint-dir
  "Open the mytool Mint app.  With \\[universal-argument], choose a screen.")

(provide 'mytool-mint)
;;; app.el ends here
