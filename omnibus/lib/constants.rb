module Constants
    module Linux
      def install_dir():
        '/opt/datadog-agent'
      end
    end

    module Windows
      def install_dir():
        'C:/opt/datadog-agent/'
      end

      def python_2_embedded_dir():
        "#{install_dir}/embedded2"
      end

      def python_3_embedded_dir():
        "#{install_dir}/embedded3"
      end
    end
  end
