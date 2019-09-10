// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog
// (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

#ifndef __UNIQUE_PTR_ADAPTER_HPP__
#define __UNIQUE_PTR_ADAPTER_HPP__

namespace dd
{
    namespace details
    {
        template <class From, class To>
        struct ptr_converter
        {
            static To convert(From t)
            {
                return t;
            }
        };
    }

    /**
     * \brief An adapter for a smart pointer (e.g. std::unique_ptr) to Win32 API that
     * allocate memory by referencing memory directly (e.g. pointer to pointer).
     * \tparam Ptr The smart pointer type
     * \tparam RawPtr The underlying raw pointer type
     * \tparam Converter A converter type to convert between RawPtr and the pointer type of Ptr
     */
    template
    <
        class Ptr,
        class RawPtr = typename Ptr::pointer,
        class Converter = details::ptr_converter<RawPtr, typename Ptr::pointer>
    >
    class unique_ptr_adapter
    {
    public:
        /**
         * \brief Stores the Ptr to initialize
         * \param uniquePtr The smart pointer to initialize
         */
        explicit unique_ptr_adapter(Ptr& uniquePtr)
        : _uniquePtr(uniquePtr)
        , _pointer(nullptr)
        {

        }

        unique_ptr_adapter(unique_ptr_adapter const&) = delete;
        unique_ptr_adapter(unique_ptr_adapter&&) = delete;
        unique_ptr_adapter& operator=(unique_ptr_adapter const&) = delete;
        unique_ptr_adapter& operator=(unique_ptr_adapter&&) = delete;

        /**
         * \brief The destructor will store the RawPtr in the Ptr and perform any conversions
         * if necessary.
         */
        ~unique_ptr_adapter()
        {
            _uniquePtr.reset(Converter::convert(_pointer));
        }

        /**
         * \brief Overload the & operator to provide an address that can be used by Win32 API
         * to store RawPtr.
         * \return The address of the RawPtr
         */
        RawPtr* operator&() // NOLINT(google-runtime-operator)
        {
            return &_pointer;
        }

    private:
        Ptr& _uniquePtr;
        RawPtr _pointer;
    };

    template <class T>
    struct ptr_traits
    {
        typedef std::unique_ptr<T> unique_ptr;
        typedef unique_ptr_adapter<unique_ptr, T*> store_ptr;
    };
}

#endif
