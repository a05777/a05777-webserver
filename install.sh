#!/bin/bash

# =================================================
# 软件许可及免责声明
# =================================================
show_disclaimer() {
    cat <<EOF
软件许可及免责声明
版权所有 (c) 2026 a05777。保留所有权利。

1. 权利保留与授权范围 (Rights & Scope)
本软件的所有权、知识产权及源代码权利均归原作者所有。获得源代码的人员（“用户”）仅拥有非营利性的个人学习、研究及调试代码的非排他性权利。未经原作者明确书面许可，严禁将本软件或其任何部分用于商业盈利、集成至商业产品、或通过网络对外提供付费/免费服务（如 SaaS/API）。

2. 绝对免责声明 (No Warranty)
本程序按“原样”（"AS IS"）提供，不附带任何形式的明示或暗示保证。作者不保证程序符合特定用途，亦不保证运行过程中不出现错误。

3. 责任限制 (Limitation of Liability)
在任何情况下，作者不对因使用本程序产生的任何损害（包括数据丢失、系统崩溃、法律诉讼等）承担任何责任。作者的全部赔偿责任上限在任何情况下均不超过用户实际支付的授权费用（如有）。

4. 风险告知与解释权 (Acceptance & Interpretation)
用户一旦运行、调试或以任何方式使用本程序，即视为完全理解并接受上述条款。作者保留对本协议的最终解释权，并有权随时更新授权条款。
EOF
    echo
    read -p "您是否同意上述声明? (y/n): " agreement
    [[ "$agreement" != "y" && "$agreement" != "Y" ]] && echo "退出。" && exit 1
}

show_disclaimer

# -------------------------------------------------
# 核心逻辑
# -------------------------------------------------

# 1. 目录处理 (防止重名冲突)
if [[ "$(basename "$(pwd)")" == "webser" ]]; then
    WORKING_DIR=$(pwd)
else
    mkdir -p webser
    cd webser || exit
    WORKING_DIR=$(pwd)
fi

# 2. 用户输入
while true; do
    read -p "是否启用SSL (true/false): " user_ssl
    if [[ "$user_ssl" == "true" || "$user_ssl" == "false" ]]; then break; else echo "请输入 true 或 false"; fi
done

read -p "请输入 HTTP 端口: " http_port
read -p "请输入 HTTPS 端口: " https_port
read -p "请输入你的域名: " domain

# 3. 处理证书逻辑
mkdir -p html ssl

# 无论用户选什么，JSON 里的 enable_ssl 始终强制为 true
final_ssl="true"

if [[ "$user_ssl" == "false" ]]; then
    echo "检测到您未开启 SSL，正在为您生成自签证书以保证程序正常启动..."
    # 检查 openssl 是否安装
    if command -v openssl >/dev/null 2>&1; then
        openssl req -x509 -newkey rsa:2048 -keyout ssl/server.key -out ssl/server.crt -days 365 -nodes -subj "/CN=$domain" > /dev/null 2>&1
        echo "自签证书 (server.crt/server.key) 已存入 ssl 目录。"
    else
        echo "警告：系统中未找到 openssl 命令，无法生成自签证书，请手动提供证书文件。"
    fi
fi

# 4. 创建 config.json
cat <<EOF > config.json
{
  "enable_ssl": $final_ssl,
  "http_port": "$http_port",
  "https_port": "$https_port",
  "domain": "$domain"
}
EOF

# 5. 下载并授权
echo "正在下载主程序..."
curl -L -o webser-bin "https://node1-rn.a05777.uk:8443/webser/dows/go/webser"
chmod +x webser-bin

# 6. 欢迎页面
cat <<EOF > html/index.html
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"><title>Webser 部署成功，如无法访问请手动启动（适用于未选择守护进程）或输入”systemctl status webser“，如未启动请输入”systemctl start webser“（适用于选择了系统守护进程）</title></head>
<body style="font-family:sans-serif; text-align:center; padding-top:100px; background:#f4f4f4;">
    <div style="display:inline-block; background:#fff; padding:50px; border-radius:15px; box-shadow:0 10px 25px rgba(0,0,0,0.1);">
        <h1 style="color:#2c3e50;">已就绪</h1>
        <p>域名: <strong>$domain</strong></p>
        <p>模式: <span style="color:#27ae60;">HTTPS</span></p>
        <p style="color:#7f8c8d; font-size:12px;">© 2026 a05777.</p>
    </div>
</body>
</html>
EOF

# 7. Systemd 守护进程
echo
read -p "是否配置 systemctl 守护进程? (y/n): " need_systemd
if [[ "$need_systemd" == "y" || "$need_systemd" == "Y" ]]; then
    SERVICE_FILE="/etc/systemd/system/webser.service"
    sudo bash -c "cat <<EOF > $SERVICE_FILE
[Unit]
Description=Webser Service
After=network.target

[Service]
Type=simple
User=$(whoami)
WorkingDirectory=$WORKING_DIR
ExecStart=$WORKING_DIR/webser-bin
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF"
    sudo systemctl daemon-reload
    read -p "是否设置开机自启? (y/n): " auto_start
    if [[ "$auto_start" == "y" || "$auto_start" == "Y" ]]; then
        sudo systemctl enable webser
        sudo systemctl start webser
    fi
fi

echo "-----------------------------------"
echo "部署完成！"
echo "程序位置: $WORKING_DIR/webser-bin"
echo "您可以直接运行: ./webser-bin"
