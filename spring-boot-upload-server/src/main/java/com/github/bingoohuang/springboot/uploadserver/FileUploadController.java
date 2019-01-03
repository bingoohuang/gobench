package com.github.bingoohuang.springboot.uploadserver;


import lombok.SneakyThrows;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.multipart.MultipartFile;

import java.io.File;
import java.util.UUID;

@RestController
public class FileUploadController {
    static {
        new File("/tmp/springbootupload/").mkdirs();
    }

    @PostMapping("/upload") @SneakyThrows
    public String handleFileUpload(@RequestParam("file") MultipartFile file) {
        File tmpFile = new File("/tmp/springbootupload/" + UUID.randomUUID().toString());
        file.transferTo(tmpFile);
        return "ok";
    }

}