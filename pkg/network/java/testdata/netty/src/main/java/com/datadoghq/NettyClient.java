package com.datadoghq;
// Need to be compiled with java7
// Original : https://github.com/netty/netty/blob/4.1/example/src/main/java/io/netty/example/http/snoop/HttpSnoopClient.java

import io.netty.bootstrap.Bootstrap;
import io.netty.buffer.Unpooled;
import io.netty.channel.Channel;
import io.netty.channel.EventLoopGroup;
import io.netty.channel.nio.NioEventLoopGroup;
import io.netty.channel.socket.nio.NioSocketChannel;
import io.netty.handler.codec.http.DefaultFullHttpRequest;
import io.netty.handler.codec.http.HttpHeaderNames;
import io.netty.handler.codec.http.HttpHeaderValues;
import io.netty.handler.codec.http.HttpMethod;
import io.netty.handler.codec.http.HttpRequest;
import io.netty.handler.codec.http.HttpVersion;
import io.netty.handler.codec.http.cookie.ClientCookieEncoder;
import io.netty.handler.codec.http.cookie.DefaultCookie;
import io.netty.handler.ssl.SslContext;
import io.netty.handler.ssl.SslContextBuilder;
import io.netty.handler.ssl.util.InsecureTrustManagerFactory;
import io.netty.handler.ssl.SslProvider;

import java.net.URI;

public final class NettyClient {
    static final String URL = "https://httpbin.org/anything/get/java-netty-test";

    public static void main(String[] args) throws Exception {
        SslProvider sslengine = SslProvider.JDK;

        for (String arg : args){
            //we only parse the arguments of the form "arg=value" (e.g: dd.debug.enabled=true)
            String[] keyValTuple = arg.split("=");
            if ((keyValTuple.length == 2) && (keyValTuple[0].equals("sslengine"))) {
                if (keyValTuple[1].equals("jdk")) {
                    sslengine = SslProvider.JDK;
                }
                if (keyValTuple[1].equals("openssl")) {
                    sslengine = SslProvider.OPENSSL;
                }
                if (keyValTuple[1].equals("openssl_refcnt")) {
                    sslengine = SslProvider.OPENSSL_REFCNT;
                }
            }
        }

        if (sslengine == SslProvider.JDK) {
            // we need to wait the agent injection here
            try {
                Thread.sleep(11*1000);
            } catch (Exception ex) {
                System.out.println(ex);
            }
        }

        URI uri = new URI(URL);
        String scheme = uri.getScheme() == null? "http" : uri.getScheme();
        String host = uri.getHost() == null? "127.0.0.1" : uri.getHost();
        int port = uri.getPort();
        if (port == -1) {
            if ("http".equalsIgnoreCase(scheme)) {
                port = 80;
            } else if ("https".equalsIgnoreCase(scheme)) {
                port = 443;
            }
        }

        // Configure SSL context if necessary.
        final boolean ssl = "https".equalsIgnoreCase(scheme);
        final SslContext sslCtx;
        if (ssl) {
            // provider JDK, OPENSSL, OPENSSL_REFCNT
            sslCtx = SslContextBuilder.forClient()
                .trustManager(InsecureTrustManagerFactory.INSTANCE)
                .sslProvider(sslengine).build();
        } else {
            sslCtx = null;
        }

        // Configure the client.
        EventLoopGroup group = new NioEventLoopGroup();
        try {
            Bootstrap b = new Bootstrap();
            b.group(group)
             .channel(NioSocketChannel.class)
             .handler(new NettyClientInitializer(sslCtx));

            // Make the connection attempt.
            Channel ch = b.connect(host, port).sync().channel();
            // Prepare the HTTP request.
            HttpRequest request = new DefaultFullHttpRequest(
                    HttpVersion.HTTP_1_1, HttpMethod.GET, uri.getRawPath(), Unpooled.EMPTY_BUFFER);
            request.headers().set(HttpHeaderNames.HOST, host);
            request.headers().set(HttpHeaderNames.CONNECTION, HttpHeaderValues.CLOSE);
            request.headers().set(HttpHeaderNames.ACCEPT_ENCODING, HttpHeaderValues.GZIP);
            
            // Send the HTTP request.
            ch.writeAndFlush(request);
            // Wait for the server to close the connection.
            ch.closeFuture().sync();
        } finally {
            // Shut down executor threads to exit.
            group.shutdownGracefully();
        }
    }
}
