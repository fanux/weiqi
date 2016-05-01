package weiqi

import (
	"net/http"
	"github.com/dgf1988/weiqi/h"
	"mime/multipart"
	"fmt"
	"image/png"
	"image"
	"image/jpeg"
	"image/gif"
	"bytes"
	"io"
	"time"
	"database/sql"
)

func img_list_handler(w http.ResponseWriter, r *http.Request, args []string) {
	var user = getSessionUser(r)
	if user == nil {
		h.SeeOther(w, r, "/login")
		return
	}
	var err error
	var data = defData()
	data.User = user
	data.Head.Title = "图片管理"
	data.Header.Navs = userNavItems()

	var imgs = make([]Img, 0)
	if rows, err := Db.Img.ListDesc(40, 0); err != nil {
		h.ServerError(w, err)
		return
	} else {
		defer rows.Close()
		for rows.Next() {
			var img Img
			if err = rows.Struct(&img); err != nil {
				h.ServerError(w, err)
				return
			} else {
				imgs = append(imgs, img)
			}
		}
		if err = rows.Err(); err != nil {
			h.ServerError(w, err)
			return
		}
	}

	data.Content["Imgs"] = imgs

	if err = defHtmlLayout().Append(defHtmlHead(), defHtmlHeader(), defHtmlFooter(), newHtmlContent("userimg")).Execute(w, data, nil); err != nil {
		logError(err.Error())
		return
	}
}

func img_editor_handler( w http.ResponseWriter, r *http.Request, args []string) {
	var user = getSessionUser(r)
	if user == nil {
		h.SeeOther(w, r, "/login")
		return
	}
	var err error
	var id = atoi64(args[0])
	var img Img
	if err := Db.Img.Get(id).Struct(&img); err == sql.ErrNoRows {
		h.NotFound(w, "找不到图片")
		return
	} else if err != nil {
		h.ServerError(w, err)
		return
	}

	if r.Method == POST {
		r.ParseForm()
		var title = r.FormValue("title")
		if title == "" {
			h.NotFound(w, "标题不能为空")
			return
		}
		if _, err = Db.Img.Update(id).Values(nil, title); err != nil {
			h.ServerError(w, err)
			return
		}
		h.SeeOther(w, r, "/user/img/"+args[0])
		return
	}
	var data = defData()
	data.User = user
	data.Head.Title = "编辑图片"
	data.Header.Navs = userNavItems()
	data.Content["Img"] = img
	var html = defHtmlLayout().Append(
		defHtmlHead(), defHtmlHeader(), defHtmlFooter(), newHtmlContent("userimgeditor"),
	)
	if err = html.Execute(w, data, nil); err != nil {
		h.ServerError(w, err)
		return
	}
}

func img_upload_handler(w http.ResponseWriter, r *http.Request, args []string) {
	if getSessionUser(r) == nil {
		h.SeeOther(w, r, "/login")
		return
	}
	//保存错误信息，提前申请一个对象，以后不用重复申请。
	var err error

	//解析POST对象文件。
	if err = r.ParseMultipartForm(2<<20); err != nil {
		h.ServerError(w, err)
		return
	}

	if r.FormValue("title") == "" {
		h.NotFound(w, "标题不能为空")
		return
	}
	//保存POST文件对象
	var srcf multipart.File
	//保存POST文件信息对象
	var header *multipart.FileHeader
	//获取POST源文件，源文件信息。
	if srcf, header, err = r.FormFile("file"); err != nil {
		h.ServerError(w, err)
		return
	} else {
		//注册关闭文件。
		defer srcf.Close()
		//从源文件头中解析可能的文件类型。
		var imgtype = parseImgType(header.Header.Get("content-type"))
		//保存图片信息
		var imgconf image.Config
		//只支持三种图片类型
		switch imgtype {
		case c_img_png:
			if imgconf, err = png.DecodeConfig(srcf); err != nil {
				h.ServerError(w, err)
				return
			}
		case c_img_jpeg:
			if imgconf, err = jpeg.DecodeConfig(srcf); err != nil {
				h.ServerError(w, err)
				return
			}
		case c_img_gif:
			if imgconf, err = gif.DecodeConfig(srcf); err != nil {
				h.ServerError(w, err)
				return
			}
		default:
			//不支持的图片类型
			h.NotFound(w, "Unsupported file type: " + header.Filename)
			return
		}

		//重置源文件读写针。
		if _, err = srcf.Seek(0, 0); err != nil {
			h.ServerError(w, err)
			return
		}
		//申请缓存
		var buff = &bytes.Buffer{}
		//读取源文件到缓存。
		if _, err = io.Copy(buff, srcf); err != nil {
			h.ServerError(w, err)
			return
		}
		//从缓存中取出bytes流。
		var imgbytes = buff.Bytes()

		//图片信息结构体
		var img Img
		//标题
		img.Title = r.FormValue("title")
		//上传文件时用的文件名。
		img.Name = header.Filename
		//图片md5
		img.Md5 = md5Bytes(imgbytes)
		img.Width = int64(imgconf.Width)
		img.Height = int64(imgconf.Height)
		//类型
		img.Type = int64(imgtype)
		//当前上传时间
		img.Upload = time.Now()

		var getimg = new(Img)
		//检查文件是否已经存在。
		if err = Db.Img.Get(nil, nil, nil, img.Md5).Struct(getimg); err == sql.ErrNoRows {
			//向本地保存文件
			if err = addFile(img.GetFullname(), imgbytes); err != nil {
				h.ServerError(w, err)
				return
			}
			//保存图片信息到数据库。
			var id int64
			if id, err = Db.Img.Add(nil, img.Title, img.Name, img.Md5, img.Width, img.Height, img.Type, nil, img.Upload); err != nil {
				h.ServerError(w, err)
				return
			}
			h.SeeOther(w, r, fmt.Sprint("/user/img/", id))
			return
		} else if err == nil {
			h.SeeOther(w, r, fmt.Sprint("/user/img/", getimg.Id))
			return
		} else {
			h.ServerError(w, err)
			return
		}
	}
}

func img_remove_handler(w http.ResponseWriter, r *http.Request, args []string) {
	if getSessionUser(r) == nil {
		h.SeeOther(w, r, "/login")
		return
	}
	r.ParseForm()
	var id = atoi64(r.FormValue("id"))
	var img Img
	if err := Db.Img.Get(id).Struct(&img); err != nil {
		h.ServerError(w, err)
		return
	} else {
		if err = removeFile(img.GetFullname()); err != nil {
			h.ServerError(w, err)
			return
		}
		if _, err = Db.Img.Del(id); err != nil {
			h.ServerError(w, err)
			return
		}
	}
	h.SeeOther(w, r, "/user/img/")
}
